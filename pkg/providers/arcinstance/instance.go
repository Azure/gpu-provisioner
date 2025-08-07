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

package arcinstance

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/hybridcontainerservice/armhybridcontainerservice"
	"github.com/azure/gpu-provisioner/pkg/providers"
	"github.com/azure/gpu-provisioner/pkg/utils"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

var (
	AgentPoolNameRegex = regexp.MustCompile(`^[a-z][a-z0-9]{0,11}$`)
)

type Provider struct {
	azClient *AZClient

	kubeClient client.Client

	subscriptionID string // Subscription ID is not directly available in the hybrid container service API, but we can get it from the auth config

	resourceGroup string

	clusterName string
}

// getConnectedClusterResourceURI constructs the resource URI for the connected cluster

func (p *Provider) getConnectedClusterResourceURI() string {

	// For Azure Hybrid Container Service, we need to construct the connected cluster resource URI

	// Format: /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Kubernetes/connectedClusters/{clusterName}

	// However, since we don't have the subscription ID directly available here,

	// we'll need to get it from the AZ client configuration. For now, let's construct it based on resourceGroup and clusterName

	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Kubernetes/connectedClusters/%s",

		p.subscriptionID, p.resourceGroup, p.clusterName)

}

func NewProvider(

	azClient *AZClient,

	kubeClient client.Client,

	subscriptionID string,

	resourceGroup string,

	clusterName string,

) *Provider {

	return &Provider{

		azClient: azClient,

		kubeClient: kubeClient,

		subscriptionID: subscriptionID,

		resourceGroup: resourceGroup,

		clusterName: clusterName,
	}

}

// Create an instance given the constraints.

// instanceTypes should be sorted by priority for spot capacity type.

func (p *Provider) Create(ctx context.Context, nodeClaim *karpenterv1.NodeClaim) (*providers.Instance, error) {

	klog.InfoS("Instance.Create", "nodeClaim", klog.KObj(nodeClaim))

	// We made a strong assumption here. The nodeClaim name should be a valid agent pool name without "-".

	apName := nodeClaim.Name

	if !AgentPoolNameRegex.MatchString(apName) {

		//https://learn.microsoft.com/en-us/troubleshoot/azure/azure-kubernetes/aks-common-issues-faq#what-naming-restrictions-are-enforced-for-aks-resources-and-parameters-

		return nil, fmt.Errorf("agentpool name(%s) is invalid, must match regex pattern: ^[a-z][a-z0-9]{0,11}$", apName)

	}

	var ap *armhybridcontainerservice.AgentPool

	err := retry.OnError(retry.DefaultBackoff, func(err error) bool {

		return false

	}, func() error {

		instanceTypes := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...).Get("node.kubernetes.io/instance-type").Values()

		if len(instanceTypes) == 0 {

			return fmt.Errorf("nodeClaim spec has no requirement for instance type")

		}

		vmSize := instanceTypes[0]

		apObj, apErr := newAgentPoolObject(vmSize, nodeClaim)

		if apErr != nil {

			return apErr

		}

		logging.FromContext(ctx).Debugf("creating Agent pool %s (%s)", apName, vmSize)

		var err error

		ap, err = createAgentPool(ctx, p.azClient.agentPoolsClient, p.getConnectedClusterResourceURI(), apName, apObj)

		if err != nil {

			switch {

			case strings.Contains(err.Error(), "Operation is not allowed because there's an in progress create node pool operation"):

				// when gpu-provisioner restarted after crash for unknown reason, we may come across this error that agent pool creating

				// is in progress, so we just need to wait node ready based on the apObj.

				ap = &apObj

				return nil

			default:

				logging.FromContext(ctx).Errorf("failed to create agent pool for nodeclaim(%s), %v", nodeClaim.Name, err)

				return fmt.Errorf("agentPool.BeginCreateOrUpdate for %q failed: %w", apName, err)

			}

		}

		logging.FromContext(ctx).Debugf("created agent pool %s", *ap.ID)

		return nil

	})

	if err != nil {

		return nil, err

	}

	instance, err := p.fromRegisteredAgentPoolToInstance(ctx, ap)

	if instance == nil && err == nil {

		// means the node object has not been found yet, we wait until the node is created

		b := wait.Backoff{

			Steps: 15,

			Duration: 1 * time.Second,

			Factor: 1.0,

			Jitter: 0.1,
		}

		err = retry.OnError(b, func(err error) bool {

			return true

		}, func() error {

			var e error

			instance, e = p.fromRegisteredAgentPoolToInstance(ctx, ap)

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

// ParseAgentPoolNameFromID parses the id stored on the instance ID

func ArcParseAgentPoolNameFromID(id string) (string, error) {

	///e.g. moc://kaito-c93a5c39-gpuvmv1-md-dq8c8-ntvb7

	// Pattern: moc://<cluster>-<hash>-<agentpool>-md-<hash>-<hash>

	r := regexp.MustCompile(`moc://[^-]+-[^-]+-(?P<AgentPoolName>[^-]+)-md-[^-]+-[^-]+`)

	matches := r.FindStringSubmatch(id)

	if matches == nil {

		return "", fmt.Errorf("id does not match the regex for ParseAgentPoolNameFromID %s", id)

	}

	for i, name := range r.SubexpNames() {

		if name == "AgentPoolName" {

			agentPoolName := matches[i]

			if agentPoolName == "" {

				return "", fmt.Errorf("cannot parse agentpool name for ParseAgentPoolNameFromID %s", id)

			}

			return agentPoolName, nil

		}

	}

	return "", fmt.Errorf("error while parsing id %s", id)

}

func (p *Provider) Get(ctx context.Context, id string) (*providers.Instance, error) {

	klog.InfoS("Instance.Get", "id", id)

	apName, err := ArcParseAgentPoolNameFromID(id)

	if err != nil {

		return nil, fmt.Errorf("getting agentpool name, %w", err)

	}

	apObj, err := getAgentPool(ctx, p.azClient.agentPoolsClient, p.getConnectedClusterResourceURI(), apName)

	if err != nil {

		if strings.Contains(err.Error(), "Agent Pool not found") || strings.Contains(err.Error(), "ResourceNotFound") || strings.Contains(err.Error(), "could not be found") {

			return nil, cloudprovider.NewNodeClaimNotFoundError(err)

		}

		logging.FromContext(ctx).Errorf("Get agentpool %q failed: %v", apName, err)

		return nil, fmt.Errorf("agentPool.Get for %s failed: %w", apName, err)

	}

	return p.convertAgentPoolToInstance(ctx, apObj, id)

}

func (p *Provider) List(ctx context.Context) ([]*providers.Instance, error) {

	klog.InfoS("Instance.List")

	apList, err := listAgentPools(ctx, p.azClient.agentPoolsClient, p.getConnectedClusterResourceURI())

	if err != nil {

		logging.FromContext(ctx).Errorf("Listing agentpools failed: %v", err)

		return nil, fmt.Errorf("agentPool.NewListPager failed: %w", err)

	}

	instances, err := p.fromAPListToInstances(ctx, apList)

	return instances, cloudprovider.IgnoreNodeClaimNotFoundError(err)

}

func (p *Provider) Delete(ctx context.Context, apName string) error {

	klog.InfoS("Instance.Delete", "agentpool name", apName)

	err := deleteAgentPool(ctx, p.azClient.agentPoolsClient, p.getConnectedClusterResourceURI(), apName)

	if err != nil {

		logging.FromContext(ctx).Errorf("Deleting agentpool %q failed: %v", apName, err)

		return fmt.Errorf("agentPool.Delete for %q failed: %w", apName, err)

	}

	return nil

}

func (p *Provider) convertAgentPoolToInstance(ctx context.Context, apObj *armhybridcontainerservice.AgentPool, id string) (*providers.Instance, error) {

	if apObj == nil || len(id) == 0 {

		return nil, fmt.Errorf("agent pool or provider id is nil")

	}

	instanceLabels := lo.MapValues(apObj.Properties.NodeLabels, func(k *string, _ string) string {

		return lo.FromPtr(k)

	})

	return &providers.Instance{

		Name: apObj.Name,

		ID: to.Ptr(id),

		Type: apObj.Properties.VMSize,

		SubnetID: nil, // VnetSubnetID not available in hybrid container service

		Tags: apObj.Tags, // Tags moved to top level

		State: (*string)(apObj.Properties.ProvisioningState), // Convert ResourceProvisioningState to string

		Labels: instanceLabels,

		ImageID: nil, // NodeImageVersion not available in hybrid container service

	}, nil

}

func (p *Provider) fromRegisteredAgentPoolToInstance(ctx context.Context, apObj *armhybridcontainerservice.AgentPool) (*providers.Instance, error) {

	if apObj == nil {

		return nil, fmt.Errorf("agent pool is nil")

	}

	nodes, err := p.getNodesByName(ctx, lo.FromPtr(apObj.Name))

	if err != nil {

		return nil, err

	}

	if len(nodes) == 0 || len(nodes) > 1 {

		// NotFound is not considered as an error

		// and AgentPool may create more than one instance, we need to wait agentPool remove

		// the spare instance.

		return nil, nil

	}

	// we only want to resolve providerID and construct instance based on AgentPool.

	// there is no need to verify the node ready condition. so comment the following if condition.

	// if node == nil || nodeutil.GetCondition(node, v1.NodeReady).Status != v1.ConditionTrue {

	// 	// node is not found or not ready

	// 	return nil, nil

	// }

	// It's need to wait node and providerID ready when create AgentPool,

	// but there is no need to wait when termination controller lists all agentpools.

	// because termination controller garbage leaked agentpools.

	if len(nodes[0].Spec.ProviderID) == 0 {

		// provider id is not found

		return nil, nil

	}

	// tokens := strings.SplitAfter(node.Name, "-vmss") // remove the vm index "0000"

	instanceLabels := lo.MapValues(apObj.Properties.NodeLabels, func(k *string, _ string) string {

		return lo.FromPtr(k)

	})

	return &providers.Instance{

		Name: apObj.Name,

		// ID:       to.Ptr(fmt.Sprint("azure://", p.getVMSSNodeProviderID(lo.FromPtr(subID), tokens[0]))),

		ID: to.Ptr(nodes[0].Spec.ProviderID),

		Type: apObj.Properties.VMSize,

		SubnetID: nil, // VnetSubnetID not available in hybrid container service

		Tags: apObj.Tags, // Tags moved to top level

		State: (*string)(apObj.Properties.ProvisioningState), // Convert ResourceProvisioningState to string

		Labels: instanceLabels,
	}, nil

}

// fromKaitoAgentPoolToInstance is used to convert agentpool that owned by kaito to Instance, and agentPools that have no

// associated node are also included in order to garbage leaked agentPools.

func (p *Provider) fromKaitoAgentPoolToInstance(ctx context.Context, apObj *armhybridcontainerservice.AgentPool) (*providers.Instance, error) {

	if apObj == nil {

		return nil, fmt.Errorf("agent pool is nil")

	}

	instanceLabels := lo.MapValues(apObj.Properties.NodeLabels, func(k *string, _ string) string {

		return lo.FromPtr(k)

	})

	ins := &providers.Instance{

		Name: apObj.Name,

		Type: apObj.Properties.VMSize,

		SubnetID: nil, // VnetSubnetID not available in hybrid container service

		Tags: apObj.Tags, // Tags moved to top level

		State: (*string)(apObj.Properties.ProvisioningState), // Convert ResourceProvisioningState to string

		Labels: instanceLabels,
	}

	nodes, err := p.getNodesByName(ctx, lo.FromPtr(apObj.Name))

	if err != nil {

		return nil, err

	}

	if len(nodes) == 1 && len(nodes[0].Spec.ProviderID) != 0 {

		ins.ID = to.Ptr(nodes[0].Spec.ProviderID)

	}

	return ins, nil

}

func (p *Provider) fromAPListToInstances(ctx context.Context, apList []*armhybridcontainerservice.AgentPool) ([]*providers.Instance, error) {

	instances := []*providers.Instance{}

	if len(apList) == 0 {

		return instances, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("agentpools not found"))

	}

	for index := range apList {

		// skip agentPool that is not owned by kaito

		if !utils.AgentPoolIsOwnedByKaito(apList[index].Properties.NodeLabels) {

			continue

		}

		// skip agentPool which is not created from nodeclaim

		if !utils.AgentPoolIsCreatedFromNodeClaim(apList[index].Properties.NodeLabels) {

			continue

		}

		instance, err := p.fromKaitoAgentPoolToInstance(ctx, apList[index])

		if err != nil {

			return instances, err

		}

		if instance != nil {

			instances = append(instances, instance)

		}

	}

	if len(instances) == 0 {

		return instances, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("agentpools not found"))

	}

	return instances, nil

}

func newAgentPoolObject(vmSize string, nodeClaim *karpenterv1.NodeClaim) (armhybridcontainerservice.AgentPool, error) {

	// Create common labels and taints

	labels := utils.CreateAgentPoolLabels(nodeClaim, vmSize)

	taintsStr := utils.CreateAgentPoolTaints(nodeClaim.Spec.Taints)

	storage := &resource.Quantity{}

	if nodeClaim.Spec.Resources.Requests != nil {

		storage = nodeClaim.Spec.Resources.Requests.Storage()

	}

	if storage.Value() <= 0 {

		return armhybridcontainerservice.AgentPool{}, fmt.Errorf("storage request of nodeclaim(%s) should be more than 0", nodeClaim.Name)

	}

	// Note: OSDiskSizeGB not supported in hybrid container service API, so we don't use diskSizeGB

	return armhybridcontainerservice.AgentPool{

		Properties: &armhybridcontainerservice.AgentPoolProperties{

			NodeLabels: labels,

			NodeTaints: taintsStr,

			VMSize: to.Ptr(vmSize),

			OSType: to.Ptr(armhybridcontainerservice.OsTypeLinux),

			Count: to.Ptr(int32(1)),

			// Note: OSDiskSizeGB not available in hybrid container service API

		},
	}, nil

}

func (p *Provider) getNodesByName(ctx context.Context, apName string) ([]*v1.Node, error) {

	nodeList := &v1.NodeList{}

	// Get all nodes first

	err := retry.OnError(retry.DefaultRetry, func(err error) bool {

		return true

	}, func() error {

		return p.kubeClient.List(ctx, nodeList)

	})

	if err != nil {

		return nil, err

	}

	// Filter nodes by matching the custom label format or standard labels

	var matchingNodes []*v1.Node

	for i := range nodeList.Items {

		node := &nodeList.Items[i]

		// Primary: Check the custom Microsoft nodepool label format: msft.microsoft/nodepool-name

		// The value format is: clusterName-randomChars-agentPoolName

		if nodepoolName, exists := node.Labels["msft.microsoft/nodepool-name"]; exists {

			// Check if the nodepool name ends with the agent pool name

			if strings.HasSuffix(nodepoolName, "-"+apName) {

				// Additional validation: check if it starts with cluster name (if available)

				if strings.HasPrefix(nodepoolName, p.clusterName+"-") {

					matchingNodes = append(matchingNodes, node)

				}

			}

		}

	}

	return matchingNodes, nil

}
