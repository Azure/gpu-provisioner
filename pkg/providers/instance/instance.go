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
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/pkg/logging"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	corecloudprovider "github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/scheduling"
	"github.com/gpu-vmprovisioner/pkg/cache"
	"github.com/gpu-vmprovisioner/pkg/providers/instancetype"

	"github.com/gpu-vmprovisioner/pkg/apis/v1alpha1"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
)

type Provider struct {
	location             string
	azClient             *AZClient
	instanceTypeProvider *instancetype.Provider
	resourceGroup        string
	subnetID             string
	clusterName          string
	unavailableOfferings *cache.UnavailableOfferings
}

func NewProvider(
	azClient *AZClient,
	instanceTypeProvider *instancetype.Provider,
	offeringsCache *cache.UnavailableOfferings,
	location string,
	resourceGroup string,
	subnetID string,
	clusterName string,
) *Provider {
	return &Provider{
		azClient:             azClient,
		instanceTypeProvider: instanceTypeProvider,
		location:             location,
		resourceGroup:        resourceGroup,
		subnetID:             subnetID,
		clusterName:          clusterName,
		unavailableOfferings: offeringsCache,
	}
}

// Create an instance given the constraints.
// instanceTypes should be sorted by priority for spot capacity type.
func (p *Provider) Create(ctx context.Context, machine *v1alpha5.Machine, instanceTypes []*corecloudprovider.InstanceType) (*Instance, error) {
	// TODO: filterInstanceTypes
	instanceTypes = orderInstanceTypesByPrice(instanceTypes, scheduling.NewNodeSelectorRequirements(machine.Spec.Requirements...))
	id, err := p.launchInstance(ctx, p.clusterName, machine, instanceTypes)
	if err != nil {
		return nil, err
	}

	return &Instance{
		ID: id,
	}, err

}

func (p *Provider) Get(ctx context.Context, id *string) (*Instance, error) {
	return &Instance{
		ID: id,
	}, nil
}

func (p *Provider) List(_ context.Context) ([]*Instance, error) {
	// Use the machine name data to determine which instances match this machine
	return nil, fmt.Errorf("not implemented")
}

func (p *Provider) Delete(ctx context.Context, id string) error {
	return nil
}

func (p *Provider) newNetworkInterfaceForVM(vmName string) armnetwork.Interface {
	return armnetwork.Interface{
		Location: to.Ptr(p.location),
		Properties: &armnetwork.InterfacePropertiesFormat{
			IPConfigurations: []*armnetwork.InterfaceIPConfiguration{
				{
					Name: &vmName,
					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
						Primary:                   to.Ptr(true),
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
						Subnet: &armnetwork.Subnet{
							ID: &p.subnetID,
						},
					},
				},
				// TODO: For Azure CNI, need to generate number of IP configs matching the max number of Pods?
				{
					Name: to.Ptr("ip1"),
					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
						Subnet: &armnetwork.Subnet{
							ID: &p.subnetID,
						},
					},
				},
				{
					Name: to.Ptr("ip2"),
					Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{
						PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
						Subnet: &armnetwork.Subnet{
							ID: &p.subnetID,
						},
					},
				},
			},
			EnableAcceleratedNetworking: to.Ptr(true),
			EnableIPForwarding:          to.Ptr(true),
		},
	}
}

func (p *Provider) createNetworkInterface(ctx context.Context, nicName string) (string, error) {
	nic := p.newNetworkInterfaceForVM(nicName)
	logging.FromContext(ctx).Debugf("Creating network interface %s", nicName)
	res, err := createNic(ctx, p.azClient.networkInterfacesClient, p.resourceGroup, nicName, nic)
	if err != nil {
		return "", err
	}
	logging.FromContext(ctx).Debugf("Successfully created network interface: %v", *res.ID)
	return *res.ID, nil
}

func newAgentPoolObject(vmSize string) armcontainerservice.AgentPool {
	scaleSetsType := armcontainerservice.AgentPoolTypeVirtualMachineScaleSets
	return armcontainerservice.AgentPool{
		Properties: &armcontainerservice.ManagedClusterAgentPoolProfileProperties{
			NodeTaints:        []*string{to.Ptr("sku=gpu:NoSchedule")},
			Type:              to.Ptr(scaleSetsType),
			VMSize:            to.Ptr(vmSize), //Standard_NC6
			EnableAutoScaling: to.Ptr(true),
			Count:             to.Ptr(int32(1)),
			MinCount:          to.Ptr(int32(1)),
			MaxCount:          to.Ptr(int32(3)),
		},
	}
}

func (p *Provider) createAgentPool(ctx context.Context, ap armcontainerservice.AgentPool, apName, clusterName string) error {
	result, err := createAgentPool(ctx, p.azClient.agentPoolsClient, p.resourceGroup, apName, clusterName, ap)
	if err != nil {
		logging.FromContext(ctx).Errorf("Creating virtual machine %q failed: %v", apName, err)
		return fmt.Errorf("agentPool.BeginCreateOrUpdate for %q failed: %w", apName, err)
	}
	logging.FromContext(ctx).Debugf("Created agent pool %s", *result.ID)
	return nil
}
func (p *Provider) launchInstance(
	ctx context.Context, clusterName string, machine *v1alpha5.Machine, instanceTypes []*corecloudprovider.InstanceType) (*string, error) {
	apName := strings.ReplaceAll("apgputest", "-", "")
	vmSize := "standard_nc12s_v3" //"standard_nc6s_v3"

	//sshPublicKey := settings.FromContext(ctx).SSHPublicKey
	ap := newAgentPoolObject(vmSize)

	logging.FromContext(ctx).Debugf("Creating Agent pool %s (%s)", apName, vmSize)
	// Uses AZ Client to create a new agent pool using the agentpool object we prepared earlier
	err := p.createAgentPool(ctx, ap, apName, clusterName)
	if err != nil {
		return nil, err
	}
	return &apName, nil
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

// pick the "best" SKU, priority and zone, from InstanceType options (and their offerings) in the request
func (p *Provider) pickSkuSizePriorityAndZone(machine *v1alpha5.Machine, instanceTypes []*corecloudprovider.InstanceType) (*corecloudprovider.InstanceType, string, string) {
	if len(instanceTypes) == 0 {
		return nil, "", ""
	}
	// InstanceType/VM SKU - just pick the first one for now. They are presorted by cheapest offering price (taking node requirements into account)
	instanceType := instanceTypes[0]
	// Priority - Provisioner defaults to Regular, so pick Spot if it is explicitly included in requirements (and is offered in at least one zone)
	priority := p.getPriorityForInstanceType(machine, instanceType)
	// Zone - ideally random/spread from zones that support given Priority
	priorityOfferings := lo.Filter(instanceType.Offerings.Available(), func(o corecloudprovider.Offering, _ int) bool { return o.CapacityType == priority })
	zonesWithPriority := lo.Map(priorityOfferings, func(o corecloudprovider.Offering, _ int) string { return o.Zone })
	zone := sets.NewString(zonesWithPriority...).UnsortedList()[0] // ~ random pick
	// Zones in Offerings have <region>-<number> format; the zone returned from here will be used for VM instantiation,
	// which expects just the zone number, without region
	zone = string(zone[len(zone)-1])

	return instanceType, priority, zone
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
