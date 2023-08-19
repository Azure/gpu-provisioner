/*
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
	"math"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/gpu-vmprovisioner/pkg/utils"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"knative.dev/pkg/logging"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	corecloudprovider "github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/scheduling"
	"github.com/gpu-vmprovisioner/pkg/cache"
	"github.com/gpu-vmprovisioner/pkg/providers/instancetype"

	"github.com/gpu-vmprovisioner/pkg/apis/v1alpha1"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
)

const (
	LabelMachineType = "gpu-provisioner.sh/machine-type"
)

type Provider struct {
	location             string
	azClient             *AZClient
	kubeClient           client.Client
	instanceTypeProvider *instancetype.Provider
	resourceGroup        string
	nodeResourceGroup    string
	subnetID             string
	clusterName          string
	unavailableOfferings *cache.UnavailableOfferings
}

func NewProvider(
	azClient *AZClient,
	kubeClient client.Client,
	instanceTypeProvider *instancetype.Provider,
	offeringsCache *cache.UnavailableOfferings,
	location string,
	resourceGroup string,
	nodeResourceGroup string,
	subnetID string,
	clusterName string,
) *Provider {
	return &Provider{
		azClient:             azClient,
		kubeClient:           kubeClient,
		instanceTypeProvider: instanceTypeProvider,
		location:             location,
		resourceGroup:        resourceGroup,
		nodeResourceGroup:    nodeResourceGroup,
		subnetID:             subnetID,
		clusterName:          clusterName,
		unavailableOfferings: offeringsCache,
	}
}

// Create an instance given the constraints.
// instanceTypes should be sorted by priority for spot capacity type.
func (p *Provider) Create(ctx context.Context, machine *v1alpha5.Machine, instanceTypes []*corecloudprovider.InstanceType) (*Instance, error) {
	var ap *armcontainerservice.AgentPool

	instanceTypes = orderInstanceTypesByPrice(instanceTypes, scheduling.NewNodeSelectorRequirements(machine.Spec.Requirements...))
	apName := strings.ReplaceAll(machine.Spec.MachineTemplateRef.Name, "-", "")
	index := 0

	err := retry.OnError(retry.DefaultBackoff, func(err error) bool {
		index++
		return instanceTypes != nil && index <= len(instanceTypes)
	}, func() error {
		instanceType := instanceTypes[index]
		capacityType := p.getPriorityForInstanceType(machine, instanceType)
		if instanceType == nil {
			return fmt.Errorf("no instance types available")
		}
		vmSize := instanceType.Name
		apObj := newAgentPoolObject(vmSize, capacityType, machine)

		logging.FromContext(ctx).Debugf("Creating Agent pool %s (%s)", apName, vmSize)
		var err error
		ap, err = createAgentPool(ctx, p.azClient.agentPoolsClient, p.resourceGroup, apName, p.clusterName, apObj)
		if err != nil {
			logging.FromContext(ctx).Errorf("Creating virtual machine %q failed: %v", apName, err)
			return fmt.Errorf("agentPool.BeginCreateOrUpdate for %q failed: %w", apName, err)
		}
		logging.FromContext(ctx).Debugf("Created agent pool %s", *ap.ID)
		return nil

	})
	if err != nil {
		return nil, err
	}
	subID, err := utils.ParseSubIDFromID(lo.FromPtr(ap.ID))
	if err != nil {
		return nil, err
	}

	node, err := p.getNodeName(ctx, lo.FromPtr(ap.Name))
	if err != nil {
		return nil, err
	}

	return p.fromAgentPoolToInstance(lo.FromPtr(subID), node.Name, ap), err
}

// GetVMSSNodeProviderID generates the provider ID for a virtual machine scale set.
func (p *Provider) GetVMSSNodeProviderID(subscriptionID, scaleSetName string) string {
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
	subID, err := utils.ParseSubIDFromID(lo.FromPtr(apObj.ID))
	if err != nil {
		return nil, err
	}
	vm, err := p.getNodeName(ctx, lo.FromPtr(apObj.Name))
	if err != nil {
		return nil, err
	}
	return p.fromAgentPoolToInstance(lo.FromPtr(subID), vm.Name, apObj), nil
}

func (p *Provider) List(ctx context.Context) ([]*Instance, error) {
	apList, err := listAgentPools(ctx, p.azClient.agentPoolsClient, p.resourceGroup, p.clusterName)
	if err != nil {
		logging.FromContext(ctx).Errorf("Listing agentpools failed: %v", err)
		return nil, fmt.Errorf("agentPool.NewListPager failed: %w", err)
	}

	var instanceList []*Instance
	for _, ap := range apList {
		subID, err := utils.ParseSubIDFromID(lo.FromPtr(ap.ID))
		if err != nil {
			return nil, err
		}
		vm, err := p.getNodeName(ctx, lo.FromPtr(ap.Name))
		if err != nil {
			return nil, err
		}
		instanceList = append(instanceList, p.fromAgentPoolToInstance(lo.FromPtr(subID), vm.Name, ap))
	}
	return instanceList, nil
}

func (p *Provider) Delete(ctx context.Context, id string) error {
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

func (p *Provider) fromAgentPoolToInstance(subscriptionID, nodeName string, apObj *armcontainerservice.AgentPool) *Instance {
	tokens := strings.SplitAfter(nodeName, "-vmss") // remove the vm index "0000"
	instanceLabels := lo.MapValues(apObj.Properties.NodeLabels, func(k *string, _ string) string {
		return lo.FromPtr(k)
	})
	return &Instance{
		Name:     apObj.Name,
		ID:       to.Ptr(fmt.Sprint("azure://", p.GetVMSSNodeProviderID(subscriptionID, tokens[0]))),
		Type:     apObj.Properties.VMSize,
		SubnetID: apObj.Properties.VnetSubnetID,
		Tags:     apObj.Properties.Tags,
		State:    apObj.Properties.ProvisioningState,
		Labels:   instanceLabels,
	}
}

func newAgentPoolObject(vmSize, capacityType string, machine *v1alpha5.Machine) armcontainerservice.AgentPool {
	taints := machine.Spec.Taints
	var taintsStr []*string
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
	}

	return armcontainerservice.AgentPool{
		Properties: &armcontainerservice.ManagedClusterAgentPoolProfileProperties{
			NodeLabels:       labels,
			NodeTaints:       taintsStr, //[]*string{to.Ptr("sku=gpu:NoSchedule")},
			Type:             to.Ptr(scaleSetsType),
			VMSize:           to.Ptr(vmSize),
			OSType:           to.Ptr(armcontainerservice.OSTypeLinux),
			Count:            to.Ptr(int32(1)), //TODO to pass it
			ScaleSetPriority: (*armcontainerservice.ScaleSetPriority)(to.Ptr(capacityType)),
		},
	}
}

// getPriorityForInstanceType selects spot if both constraints are flexible and there is an available offering.
// The Azure Cloud Provider defaults to Regular, so spot must be explicitly included in capacity type requirements.
//
// Unlike AWS getCapacityType, this picks based on a single pre-selected InstanceType, rather than all InstanceType options in nodeRequest,
// because Azure Cloud Provider does client-side selection of particular InstanceType from options
func (p *Provider) getPriorityForInstanceType(machine *v1alpha5.Machine, instanceType *corecloudprovider.InstanceType) string {
	requirements := scheduling.NewNodeSelectorRequirements(machine.
		Spec.Requirements...)

	if requirements.Get(v1alpha5.LabelCapacityType).Has(v1alpha1.PrioritySpot) {
		for _, offering := range instanceType.Offerings.Available() {
			if requirements.Get(v1.LabelTopologyZone).Has(offering.Zone) && offering.CapacityType == v1alpha1.PrioritySpot {
				return v1alpha1.PrioritySpot
			}
		}
	}
	return v1alpha1.PriorityRegular
}

func orderInstanceTypesByPrice(instanceTypes []*corecloudprovider.InstanceType, requirements scheduling.Requirements) []*corecloudprovider.InstanceType {
	// Order instance types so that we get the cheapest instance types of the available offerings
	sort.Slice(instanceTypes, func(i, j int) bool {
		iPrice := math.MaxFloat64
		jPrice := math.MaxFloat64
		if len(instanceTypes[i].Offerings.Available().Requirements(requirements)) > 0 {
			iPrice = instanceTypes[i].Offerings.Available().Requirements(requirements).Cheapest().Price
		}
		if len(instanceTypes[j].Offerings.Available().Requirements(requirements)) > 0 {
			jPrice = instanceTypes[j].Offerings.Available().Requirements(requirements).Cheapest().Price
		}
		if iPrice == jPrice {
			return instanceTypes[i].Name < instanceTypes[j].Name
		}
		return iPrice < jPrice
	})
	return instanceTypes
}

func (p *Provider) getNodeName(ctx context.Context, apName string) (*v1.Node, error) {
	nodeList := &v1.NodeList{}
	req, err := labels.NewRequirement("agentpool", selection.Equals, []string{apName})
	if err != nil {
		return nil, err
	}
	listOpts := &client.ListOptions{
		LabelSelector: labels.NewSelector().Add(lo.FromPtr(req)),
	}
	err = p.kubeClient.List(ctx, nodeList, listOpts)
	if err != nil {
		return nil, err
	}
	if nodeList == nil || len(nodeList.Items) == 0 {
		return nil, fmt.Errorf("no node has been found for the agentpool %s", apName)
	}
	return &nodeList.Items[0], nil
}
