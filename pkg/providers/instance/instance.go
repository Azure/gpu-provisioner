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

	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/pkg/logging"

	"github.com/gpu-vmprovisioner/pkg/cache"
	"github.com/gpu-vmprovisioner/pkg/providers/instancetype"
	"github.com/gpu-vmprovisioner/pkg/providers/launchtemplate"
	"github.com/gpu-vmprovisioner/pkg/utils"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	corecloudprovider "github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/scheduling"

	"github.com/gpu-vmprovisioner/pkg/apis/settings"
	"github.com/gpu-vmprovisioner/pkg/apis/v1alpha1"

	sdkerrors "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
)

type Provider struct {
	location               string
	azClient               *AZClient
	instanceTypeProvider   *instancetype.Provider
	launchTemplateProvider *launchtemplate.Provider
	resourceGroup          string
	subnetID               string
	unavailableOfferings   *cache.UnavailableOfferings
}

func NewProvider(
	_ context.Context,
	azClient *AZClient,
	instanceTypeProvider *instancetype.Provider,
	launchTemplateProvider *launchtemplate.Provider,
	offeringsCache *cache.UnavailableOfferings,
	location string,
	resourceGroup string,
	subnetID string,
) *Provider {
	return &Provider{
		azClient:               azClient,
		instanceTypeProvider:   instanceTypeProvider,
		launchTemplateProvider: launchTemplateProvider,
		location:               location,
		resourceGroup:          resourceGroup,
		subnetID:               subnetID,
		unavailableOfferings:   offeringsCache,
	}
}

// Create an instance given the constraints.
// instanceTypes should be sorted by priority for spot capacity type.
func (p *Provider) Create(ctx context.Context, nodeTemplate *v1alpha1.NodeTemplate, machine *v1alpha5.Machine, instanceTypes []*corecloudprovider.InstanceType) (*armcompute.VirtualMachine, error) {
	// TODO: filterInstanceTypes
	instanceTypes = orderInstanceTypesByPrice(instanceTypes, scheduling.NewNodeSelectorRequirements(machine.Spec.Requirements...))
	id, err := p.launchInstance(ctx, nodeTemplate, machine, instanceTypes)
	if err != nil {
		return nil, err
	}
	// Get Instance with backoff retry (TODO: check if actually needed)
	// AWS provider needs to getInstance here, because it delegates certain choices (e.g. actual instanceType to use) to Fleet API.
	// (It also .InstanceID, .PrivateDnsName, .Placement.AvailibilityZone, and .SpotInstanceRequestId - to get CapacityType)
	// A provider that does client-side choices does not need to getInstance/wait here ...
	// Unless we want to set ProviderID on v1.Node?

	logging.FromContext(ctx).Debugf("Waiting for VirtualMachines.Get(%s)", *id)
	resp, err := p.azClient.virtualMachinesClient.Get(ctx, p.resourceGroup, *id, nil)
	if err != nil {
		return nil, err
	}
	if resp.ID == nil {
		return nil, fmt.Errorf("azure virtual machine response should not have a nil id")
	}
	instance := resp.VirtualMachine
	// TODO: check instance status
	// vm.ProvisioningState
	// vm.InstanceView.Statuses
	zone, err := GetZoneID(&resp.VirtualMachine)
	if err != nil {
		logging.FromContext(ctx).Error(err)
	}
	logging.FromContext(ctx).With(
		"launched-instance", *instance.ID,
		"hostname", instance.Name,
		"type", instance.Properties.HardwareProfile.VMSize,
		"zone", zone,
		/*"capacity-type", getCapacityType(instance)*/).Infof("launched new instance")

	return &instance, nil
}

func (p *Provider) Get(ctx context.Context, id string) (*armcompute.VirtualMachine, error) {
	// TODO: AWS provider does filtering by state and call batching here, ec2.DescribeInstances supports multiple
	var vm armcompute.VirtualMachinesClientGetResponse
	var err error

	// TODO: do we need armcompute.InstanceView here?
	// TODO: check instance status
	// vm.ProvisioningState
	// vm.InstanceView.Statuses
	if vm, err = p.azClient.virtualMachinesClient.Get(ctx, p.resourceGroup, id, nil); err != nil {
		if sdkerrors.IsNotFoundErr(err) {
			return nil, corecloudprovider.NewMachineNotFoundError(err)
		}
		return nil, fmt.Errorf("failed to get VM instance, %w", err)
	}

	return &vm.VirtualMachine, nil
}

func (p *Provider) List(_ context.Context) ([]*armcompute.VirtualMachine, error) {
	// Use the machine name data to determine which instances match this machine
	return nil, fmt.Errorf("not implemented")
}

func (p *Provider) Delete(ctx context.Context, machine *v1alpha5.Machine) error {
	id, err := utils.ParseInstanceID(machine.Status.ProviderID)
	if err != nil {
		return fmt.Errorf("getting instance ID, %w", err)
	}

	// TODO: retries
	// TODO: Q: there is no API for deleting multiple VMs in one call?
	logging.FromContext(ctx).Debugf("Deleting virtual machine %s", id)
	return deleteVirtualMachine(ctx, p.azClient.virtualMachinesClient, p.resourceGroup, id)
}

func (p *Provider) createBillingExtension(
	ctx context.Context,
	vmName, _ string,
) error {
	var err error
	vmExt := p.getBillingExtension()
	vmExtName := *vmExt.Name
	logging.FromContext(ctx).Debugf("Creating virtual machine billing extension for %s", vmName)
	v, err := createVirtualMachineExtension(ctx, p.azClient.virtualMachinesExtensionClient, p.resourceGroup, vmName, vmExtName, *vmExt)
	if err != nil {
		logging.FromContext(ctx).Errorf("Creating VM billing extension for VM %q failed, %w", vmName, err)
		vmErr := deleteVirtualMachine(ctx, p.azClient.virtualMachinesClient, p.resourceGroup, vmName)
		if vmErr != nil {
			logging.FromContext(ctx).Errorf("virtualMachine.Delete for %s failed: %v", vmName, vmErr)
		}
		return fmt.Errorf("creating VM billing extension for VM %q, %w failed", vmName, err)
	}
	logging.FromContext(ctx).Debugf("Created  virtual machine billing extension for %s, with an id of %s", vmName, *v.ID)
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

func generateVMName(machineName string) string {
	return fmt.Sprintf("aks-%s", machineName)
}

func (p *Provider) createNetworkInterface(ctx context.Context, nicName string, launchTemplateConfig *launchtemplate.Template) (string, error) {
	nic := p.newNetworkInterfaceForVM(nicName)
	p.applyTemplateToNic(&nic, launchTemplateConfig)
	logging.FromContext(ctx).Debugf("Creating network interface %s", nicName)
	res, err := createNic(ctx, p.azClient.networkInterfacesClient, p.resourceGroup, nicName, nic)
	if err != nil {
		return "", err
	}
	logging.FromContext(ctx).Debugf("Successfully created network interface: %v", *res.ID)
	return *res.ID, nil
}

func newVMObject(
	vmName, nicReference, vmSize, zone, capacityType string,
	location string,
	sshPublicKey string) armcompute.VirtualMachine {
	return armcompute.VirtualMachine{
		Location: to.Ptr(location),
		Properties: &armcompute.VirtualMachineProperties{
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: to.Ptr(armcompute.VirtualMachineSizeTypes(vmSize)),
			},

			// StorageProfile.ImageReference set from template
			StorageProfile: &armcompute.StorageProfile{
				OSDisk: &armcompute.OSDisk{
					Name:         to.Ptr(vmName),
					CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
					DeleteOption: to.Ptr(armcompute.DiskDeleteOptionTypesDelete),
				},
			},

			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: []*armcompute.NetworkInterfaceReference{
					{
						ID: &nicReference,
						Properties: &armcompute.NetworkInterfaceReferenceProperties{
							Primary:      to.Ptr(true),
							DeleteOption: to.Ptr(armcompute.DeleteOptionsDelete),
						},
					},
				},
			},

			OSProfile: &armcompute.OSProfile{
				AdminUsername: to.Ptr("azureuser"),
				ComputerName:  &vmName,
				LinuxConfiguration: &armcompute.LinuxConfiguration{
					DisablePasswordAuthentication: to.Ptr(true),
					SSH: &armcompute.SSHConfiguration{
						PublicKeys: []*armcompute.SSHPublicKey{
							{
								KeyData: to.Ptr(sshPublicKey),
								// TODO: parameterize this
								Path: to.Ptr("/home/" + "azureuser" + "/.ssh/authorized_keys"),
							},
						},
					},
				},

				// CustomData set from template
			},
			Priority: to.Ptr(armcompute.VirtualMachinePriorityTypes(capacityType)),
		},

		// Can optionally join an existing VMSS Flex. Not using for now ...
		//VirtualMachineScaleSet: &compute.SubResource{
		//	ID: &vmssFlexID,
		//},
		Zones: []*string{&zone},
	}
}

func (p *Provider) createVirtualMachine(ctx context.Context, vm armcompute.VirtualMachine, vmName, _ string) error {
	result, err := createVirtualMachine(ctx, p.azClient.virtualMachinesClient, p.resourceGroup, vmName, vm)
	if err != nil {
		logging.FromContext(ctx).Errorf("Creating virtual machine %q failed: %v", vmName, err)
		return fmt.Errorf("virtualMachine.BeginCreateOrUpdate for VM %q failed: %w", vmName, err)
	}
	logging.FromContext(ctx).Debugf("Created  virtual machine %s", *result.ID)
	return nil
}

func (p *Provider) launchInstance(
	ctx context.Context, nodeTemplate *v1alpha1.NodeTemplate, machine *v1alpha5.Machine, instanceTypes []*corecloudprovider.InstanceType) (*string, error) {
	//AWS:
	// capacityType := p.getCapacityType(machine, instanceTypes)
	// launchTemplateConfigs, err := p.getLaunchTemplateConfigs(ctx, nodeTemplate, machine, instanceTypes, capacityType)
	instanceType, capacityType, zone := p.pickSkuSizePriorityAndZone(machine, instanceTypes)
	if instanceType == nil {
		return nil, fmt.Errorf("no instance types available")
	}
	launchTemplate, err := p.getLaunchTemplate(ctx, nodeTemplate, machine, instanceType, capacityType)
	if err != nil {
		return nil, fmt.Errorf("getting launch template: %w", err)
	}

	vmName := generateVMName(machine.Name)

	// create network interface
	nicName := vmName
	nicReference, err := p.createNetworkInterface(ctx, vmName, launchTemplate)
	if err != nil {
		return nil, err
	}
	vmSize := instanceType.Name

	sshPublicKey := settings.FromContext(ctx).SSHPublicKey
	vm := newVMObject(vmName, nicReference, vmSize, zone, capacityType, p.location, sshPublicKey)

	// the following should enable ephemeral os disk for instance types that support it
	// TODO: this (as many other settings) should come from elsewhere
	ephemeralOSDiskRequirements := scheduling.NewRequirements(scheduling.NewRequirement(v1alpha1.LabelSKUEphemeralOSDiskSupported, v1.NodeSelectorOpIn, "true"))
	if err = instanceType.Requirements.Compatible(ephemeralOSDiskRequirements); err == nil {
		osDisk := vm.Properties.StorageProfile.OSDisk
		osDisk.DiffDiskSettings = &armcompute.DiffDiskSettings{
			Option: to.Ptr(armcompute.DiffDiskOptionsLocal),
			// Placement: compute.ResourceDisk,
		}
		osDisk.Caching = to.Ptr(armcompute.CachingTypesReadOnly)
		osDisk.DiskSizeGB = to.Ptr[int32](42)
	}

	if capacityType == v1alpha1.PrioritySpot {
		// TODO: review EvicitonPolicy; consider supporting MaxPrice
		vm.Properties.BillingProfile = &armcompute.BillingProfile{
			MaxPrice: to.Ptr(float64(-1)),
		}
	}

	// apply launch template configuration
	p.applyTemplate(&vm, launchTemplate)

	logging.FromContext(ctx).Debugf("Creating virtual machine %s (%s)", vmName, vmSize)
	// Uses AZ Client to create a new virtual machine using the vm object we prepared earlier
	err = p.createVirtualMachine(ctx, vm, vmName, nicName)
	if err != nil {
		if sdkerrors.SubscriptionQuotaHasBeenReached(err) {
			p.unavailableOfferings.MarkUnavailable(ctx, "SubscriptionLevelQuotaReached", instanceType.Name, zone, capacityType)
		}
		return nil, err
	}

	// billing extension
	// TODO: consider making this async (with provisioner not having to wait)
	err = p.createBillingExtension(ctx, vmName, nicName)
	if err != nil {
		return nil, err
	}
	return &vmName, nil
}

func (p *Provider) applyTemplateToNic(nic *armnetwork.Interface, template *launchtemplate.Template) {
	// set tags
	nic.Tags = template.Tags
}

func (p *Provider) applyTemplate(vm *armcompute.VirtualMachine, template *launchtemplate.Template) {
	// set tags
	vm.Tags = template.Tags

	// set image reference
	vm.Properties.StorageProfile = &armcompute.StorageProfile{
		ImageReference: &armcompute.ImageReference{
			CommunityGalleryImageID: to.Ptr(template.ImageID),
		},
	}

	// set custom data
	vm.Properties.OSProfile.CustomData = to.Ptr(template.UserData)

	// TODO: externalize hardcoded IDs
	//vmssName := "karpenter-vmss
	//vmssFlexID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachineScaleSets/%s", p.subscriptionID, resourceGroupName, vmssName)
}

func (p *Provider) getLaunchTemplate(ctx context.Context, nodeTemplate *v1alpha1.NodeTemplate, machine *v1alpha5.Machine,
	instanceType *corecloudprovider.InstanceType, capacityType string) (*launchtemplate.Template, error) {
	// TODO: set HyperVGeneration. Available from InstanceView after VM is created, but maybe there is a way to know it in advance?

	// unlike AWS provider, pass Requirements-derived labels to kubelet to account for node joining before return from cloudprovider.Create();
	// as much as possible, match v1.Node labels that karpenter-code generates from Machine, upon return from cloudprovider.Create():
	// use getAllSingleValuedRequirementLabels(instanceType) - used in instanceToMachine() to generate Machine labels that karpenter-core uses
	// TODO: should we filter restricted labels here by using instanceType.Requirements.Labels()?
	// TODO: revisit; karpenter-core also adds MachineTemplate.Labels and .Requirements.Labels()
	additionalLabels := lo.Assign(GetAllSingleValuedRequirementLabels(instanceType), map[string]string{v1alpha5.LabelCapacityType: capacityType})

	launchTemplate, err := p.launchTemplateProvider.GetTemplate(ctx, nodeTemplate, machine, instanceType, additionalLabels)
	if err != nil {
		return nil, fmt.Errorf("getting launch templates, %w", err)
	}

	return launchTemplate, nil
}

// GetAllSingleValuedRequirementLabels converts instanceType.Requirements to labels
// Like   instanceType.Requirements.Labels() it uses single-valued requirements
// Unlike instanceType.Requirements.Labels() it does not filter out restricted Node labels
func GetAllSingleValuedRequirementLabels(instanceType *corecloudprovider.InstanceType) map[string]string {
	labels := map[string]string{}
	if instanceType == nil {
		return labels
	}
	for key, req := range instanceType.Requirements {
		if req.Len() == 1 {
			labels[key] = req.Values()[0]
		}
	}
	return labels
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

// filterInstanceTypes is used to eliminate less desirable instance types (like GPUs) from the list of possible instance types when
// a set of more appropriate instance types would work. If a set of more desirable instance types is not found, then the original slice
// of instance types are returned.
// func filterInstanceTypes(instanceTypes []*cloudprovider.InstanceType) []*cloudprovider.InstanceType {
// 	// TODO
// 	return instanceTypes
// }

func GetCapacityType(instance *armcompute.VirtualMachine) string {
	if instance != nil && instance.Properties != nil && instance.Properties.Priority != nil {
		return string(*instance.Properties.Priority)
	}
	return ""
}

func GetHyperVGeneration(instance *armcompute.VirtualMachine) string {
	if instance != nil && instance.Properties != nil && instance.Properties.InstanceView != nil && instance.Properties.InstanceView.HyperVGeneration != nil {
		return string(*instance.Properties.InstanceView.HyperVGeneration)
	}
	return ""
}

func (p *Provider) getBillingExtension() *armcompute.VirtualMachineExtension {
	const (
		vmExtensionType              = "Microsoft.Compute/virtualMachines/extensions"
		aksBillingExtensionName      = "computeAksLinuxBilling"
		aksBillingExtensionPublisher = "Microsoft.AKS"
		aksBillingExtensionTypeLinux = "Compute.AKS.Linux.Billing"
	)

	vmExtension := &armcompute.VirtualMachineExtension{
		Location: to.Ptr(p.location),
		Name:     to.Ptr(aksBillingExtensionName),
		Properties: &armcompute.VirtualMachineExtensionProperties{
			Publisher:               to.Ptr(aksBillingExtensionPublisher),
			TypeHandlerVersion:      to.Ptr("1.0"),
			AutoUpgradeMinorVersion: to.Ptr(true),
			Settings:                &map[string]interface{}{},
			Type:                    to.Ptr(aksBillingExtensionTypeLinux),
		},
		Type: to.Ptr(vmExtensionType),
	}

	return vmExtension
}

func GetZoneID(vm *armcompute.VirtualMachine) (string, error) {
	if vm == nil {
		return "", fmt.Errorf("cannot pass in a nil virtual machine")
	}
	if vm.Name == nil {
		return "", fmt.Errorf("virtual machine is missing name")
	}
	if vm.Zones == nil {
		return "", fmt.Errorf("virtual machine %v zones are nil", *vm.Name)
	}
	if len(vm.Zones) == 1 {
		return *(vm.Zones)[0], nil
	}
	if len(vm.Zones) > 1 {
		return "", fmt.Errorf("virtual machine %v has multiple zones", *vm.Name)
	}
	return "", fmt.Errorf("virtual machine %v does not have any zones specified", *vm.Name)
}
