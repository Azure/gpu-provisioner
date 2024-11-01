/*
       Copyright (c) Microsoft Corporation.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package instance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/azure/gpu-provisioner/pkg/utils"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"knative.dev/pkg/logging"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/scheduling"
	"github.com/azure/gpu-provisioner/pkg/cache"
	"github.com/azure/gpu-provisioner/pkg/providers/instancetype"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	nodeutil "github.com/aws/karpenter-core/pkg/utils/node"
)

const (
	LabelMachineType = "kaito.sh/machine-type"
)

type Provider struct {
	azClient             *AZClient
	kubeClient           client.Client
	instanceTypeProvider *instancetype.Provider
	resourceGroup        string
	nodeResourceGroup    string
	clusterName          string
	unavailableOfferings *cache.UnavailableOfferings
}

func NewProvider(
	azClient *AZClient,
	kubeClient client.Client,
	instanceTypeProvider *instancetype.Provider,
	offeringsCache *cache.UnavailableOfferings,

	resourceGroup string,
	nodeResourceGroup string,
	clusterName string,
) *Provider {
	return &Provider{
		azClient:             azClient,
		kubeClient:           kubeClient,
		instanceTypeProvider: instanceTypeProvider,
		resourceGroup:        resourceGroup,
		nodeResourceGroup:    nodeResourceGroup,
		clusterName:          clusterName,
		unavailableOfferings: offeringsCache,
	}
}

// Create an instance given the constraints.
// instanceTypes should be sorted by priority for spot capacity type.
func (p *Provider) Create(ctx context.Context, machine *v1alpha5.Machine) (*Instance, error) {
	klog.InfoS("Instance.Create", "machine", klog.KObj(machine))

	// We made a strong assumption here. The machine name should be a valid agent pool name without "-".
	apName := machine.Name
	if len(apName) > 11 {
		//https://learn.microsoft.com/en-us/troubleshoot/azure/azure-kubernetes/aks-common-issues-faq#what-naming-restrictions-are-enforced-for-aks-resources-and-parameters-
		return nil, fmt.Errorf("the length agentpool name should be less than 11, got %d (%s)", len(apName), apName)
	}

	var ap *armcontainerservice.AgentPool
	err := retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return false
	}, func() error {
		instanceTypes := scheduling.NewNodeSelectorRequirements(machine.Spec.Requirements...).Get("node.kubernetes.io/instance-type").Values()
		if len(instanceTypes) == 0 {
			return fmt.Errorf("machine spec has no requirement for instance type")
		}

		vmSize := instanceTypes[0]
		apObj := newAgentPoolObject(vmSize, machine)

		logging.FromContext(ctx).Debugf("creating Agent pool %s (%s)", apName, vmSize)
		var err error
		ap, err = createAgentPool(ctx, p.azClient.agentPoolsClient, p.resourceGroup, apName, p.clusterName, apObj)
		if err != nil {
			return fmt.Errorf("agentPool.BeginCreateOrUpdate for %q failed: %w", apName, err)
		}
		logging.FromContext(ctx).Debugf("created agent pool %s", *ap.ID)
		return nil

	})
	if err != nil {
		return nil, err
	}

	instance, err := p.fromAgentPoolToInstance(ctx, ap)
	if instance == nil && err == nil {
		// means the node object has not been found yet, we wait until the node is created
		b := wait.Backoff{
			Steps:    15,
			Duration: 1 * time.Second,
			Factor:   1.0,
			Jitter:   0.1,
		}

		err = retry.OnError(b, func(err error) bool {
			return true
		}, func() error {
			var e error
			instance, e = p.fromAgentPoolToInstance(ctx, ap)
			if e != nil {
				return e
			}
			if instance == nil {
				return fmt.Errorf("fail to find the node object")
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return instance, err
}

// getVMSSNodeProviderID generates the provider ID for a virtual machine scale set.
func (p *Provider) getVMSSNodeProviderID(subscriptionID, scaleSetName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachineScaleSets/%s/virtualMachines/0", //vm = 0 as ew have the count always 1
		subscriptionID,
		strings.ToLower(p.nodeResourceGroup),
		scaleSetName,
	)
}

func (p *Provider) Get(ctx context.Context, id string) (*Instance, error) {
	apName, err := utils.ParseAgentPoolNameFromID(id)
	if err != nil {
		return nil, fmt.Errorf("getting agentpool name, %w", err)
	}
	apObj, err := getAgentPool(ctx, p.azClient.agentPoolsClient, p.resourceGroup, apName, p.clusterName)
	if err != nil {
		logging.FromContext(ctx).Errorf("Get agentpool %q failed: %v", apName, err)
		return nil, fmt.Errorf("agentPool.Get for %s failed: %w", apName, err)
	}

	return p.fromAgentPoolToInstance(ctx, apObj)
}

func (p *Provider) List(ctx context.Context) ([]*Instance, error) {
	apList, err := listAgentPools(ctx, p.azClient.agentPoolsClient, p.resourceGroup, p.clusterName)
	if err != nil {
		logging.FromContext(ctx).Errorf("Listing agentpools failed: %v", err)
		return nil, fmt.Errorf("agentPool.NewListPager failed: %w", err)
	}

	return p.fromAPListToInstances(ctx, apList)
}

func (p *Provider) Delete(ctx context.Context, id string) error {
	klog.InfoS("Instance.Delete", "id", id)

	apName, err := utils.ParseAgentPoolNameFromID(id)
	if err != nil {
		return fmt.Errorf("getting agentpool name, %w", err)
	}
	err = deleteAgentPool(ctx, p.azClient.agentPoolsClient, p.resourceGroup, apName, p.clusterName)
	if err != nil {
		logging.FromContext(ctx).Errorf("Deleting agentpool %q failed: %v", apName, err)
		return fmt.Errorf("agentPool.Delete for %q failed: %w", apName, err)
	}
	return nil
}

func (p *Provider) fromAgentPoolToInstance(ctx context.Context, apObj *armcontainerservice.AgentPool) (*Instance, error) {
	node, err := p.getNodeByName(ctx, lo.FromPtr(apObj.Name))
	if err != nil {
		return nil, err
	}
	if node == nil || nodeutil.GetCondition(node, v1.NodeReady).Status != v1.ConditionTrue {
		// node is not found or not ready
		return nil, nil
	}
	instanceLabels := lo.MapValues(apObj.Properties.NodeLabels, func(k *string, _ string) string {
		return lo.FromPtr(k)
	})
	return &Instance{
		Name:     apObj.Name,
		ID:       to.Ptr(node.Spec.ProviderID),
		Type:     apObj.Properties.VMSize,
		SubnetID: apObj.Properties.VnetSubnetID,
		Tags:     apObj.Properties.Tags,
		State:    apObj.Properties.ProvisioningState,
		Labels:   instanceLabels,
	}, nil
}

func (p *Provider) fromAPListToInstances(ctx context.Context, apList []*armcontainerservice.AgentPool) ([]*Instance, error) {
	if len(apList) == 0 {
		return nil, fmt.Errorf("no agentpools found")
	}
	instances := []*Instance{}
	for index := range apList {
		instance, err := p.fromAgentPoolToInstance(ctx, apList[index])
		if err != nil {
			return nil, err
		}
		if instance != nil { // exclude not found or not ready node
			instances = append(instances, instance)
		}
	}
	return instances, nil
}

func newAgentPoolObject(vmSize string, machine *v1alpha5.Machine) armcontainerservice.AgentPool {
	taints := machine.Spec.Taints
	taintsStr := []*string{}
	for _, t := range taints {
		taintsStr = append(taintsStr, to.Ptr(fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect)))
	}
	scaleSetsType := armcontainerservice.AgentPoolTypeVirtualMachineScaleSets
	labels := map[string]*string{v1alpha5.ProvisionerNameLabelKey: to.Ptr("default")}
	for k, v := range machine.Labels {
		labels[k] = to.Ptr(v)
	}

	if strings.Contains(vmSize, "Standard_N") {
		labels = lo.Assign(labels, map[string]*string{LabelMachineType: to.Ptr("gpu")})
	} else {
		labels = lo.Assign(labels, map[string]*string{LabelMachineType: to.Ptr("cpu")})
	}

	storage := &resource.Quantity{}
	if machine.Spec.Resources.Requests != nil {
		storage = machine.Spec.Resources.Requests.Storage()
	}

	return armcontainerservice.AgentPool{
		Properties: &armcontainerservice.ManagedClusterAgentPoolProfileProperties{
			NodeLabels:   labels,
			NodeTaints:   taintsStr, //[]*string{to.Ptr("sku=gpu:NoSchedule")},
			Type:         to.Ptr(scaleSetsType),
			VMSize:       to.Ptr(vmSize),
			OSType:       to.Ptr(armcontainerservice.OSTypeLinux),
			Count:        to.Ptr(int32(1)),
			OSDiskSizeGB: to.Ptr(int32(storage.Value())),
		},
	}
}

func (p *Provider) getNodeByName(ctx context.Context, apName string) (*v1.Node, error) {
	nodeList := &v1.NodeList{}
	labelSelector := client.MatchingLabels{"agentpool": apName, "kubernetes.azure.com/agentpool": apName}

	err := retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true
	}, func() error {
		return p.kubeClient.List(ctx, nodeList, labelSelector)
	})
	if err != nil {
		return nil, err
	}

	if nodeList == nil || len(nodeList.Items) == 0 {
		// NotFound is not considered as an error
		return nil, nil
	}

	return &nodeList.Items[0], nil
}
