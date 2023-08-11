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

package cloudprovider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// nolint SA1019 - deprecated package
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"

	"github.com/gpu-vmprovisioner/pkg/apis"
	"github.com/gpu-vmprovisioner/pkg/apis/v1alpha1"

	"github.com/gpu-vmprovisioner/pkg/providers/imagefamily"
	"github.com/gpu-vmprovisioner/pkg/providers/instance"
	"github.com/gpu-vmprovisioner/pkg/providers/instancetype"
	"github.com/gpu-vmprovisioner/pkg/utils"
	"github.com/samber/lo"

	coreapis "github.com/aws/karpenter-core/pkg/apis"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/scheduling"
	"github.com/aws/karpenter-core/pkg/utils/functional"
	"github.com/aws/karpenter-core/pkg/utils/resources"
	"sigs.k8s.io/cloud-provider-azure/pkg/provider"

	"github.com/gpu-vmprovisioner/pkg/staticprovisioner"
)

func init() {
	// TODO: add Azure aliased labels, if any
	// v1alpha5.NormalizedLabels = lo.Assign(v1alpha5.NormalizedLabels, ...)

	// change the non-spot priority/capacity name known to Karpenter core
	// from "on-demand" to "regular" (currently only affects consolidation)
	// TODO: was changed to constant, so can't override now ...
	// v1alpha5.CapacityTypeOnDemand = v1alpha1.PriorityRegular

	coreapis.Settings = append(coreapis.Settings, apis.Settings...)
}

var _ cloudprovider.CloudProvider = (*CloudProvider)(nil)

type CloudProvider struct {
	instanceTypeProvider *instancetype.Provider
	instanceProvider     *instance.Provider
	kubeClient           client.Client
	imageProvider        *imagefamily.Provider
}

func New(instanceTypeProvider *instancetype.Provider, instanceProvider *instance.Provider, kubeClient client.Client, imageProvider *imagefamily.Provider) *CloudProvider {
	return &CloudProvider{
		instanceTypeProvider: instanceTypeProvider,
		instanceProvider:     instanceProvider,
		kubeClient:           kubeClient,
		imageProvider:        imageProvider,
	}
}

// Create a node given the constraints.
func (c *CloudProvider) Create(ctx context.Context, machine *v1alpha5.Machine) (*v1alpha5.Machine, error) {
	//nodeTemplate, err := c.resolveNodeTemplate(ctx, []byte(machine.
	//	Annotations[v1alpha5.ProviderCompatabilityAnnotationKey]), machine.
	//	Spec.MachineTemplateRef)
	//if err != nil {
	//	return nil, fmt.Errorf("resolving node template, %w", err)
	//}
	instanceTypes, err := c.resolveInstanceTypes(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("resolving instance types, %w", err)
	}
	if len(instanceTypes) == 0 {
		return nil, fmt.Errorf("all requested instance types were unavailable during launch")
	}
	instance, err := c.instanceProvider.Create(ctx, machine, instanceTypes)
	if err != nil {
		return nil, fmt.Errorf("creating instance, %w", err)
	}
	instanceType, _ := lo.Find(instanceTypes, func(i *cloudprovider.InstanceType) bool {
		return i.Name == string(*instance.Properties.HardwareProfile.VMSize)
	})

	return c.instanceToMachine(ctx, instance, instanceType), nil
}

func (c *CloudProvider) List(ctx context.Context) ([]*v1alpha5.Machine, error) {
	instances, err := c.instanceProvider.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing instances, %w", err)
	}
	var machines []*v1alpha5.Machine
	for _, instance := range instances {
		instanceType, err := c.resolveInstanceTypeFromInstance(ctx, instance)
		if err != nil {
			return nil, fmt.Errorf("resolving instance type, %w", err)
		}
		machines = append(machines, c.instanceToMachine(ctx, instance, instanceType))
	}
	return machines, nil
}

func (c *CloudProvider) Get(ctx context.Context, providerID string) (*v1alpha5.Machine, error) {
	id, err := utils.ParseInstanceID(providerID)
	if err != nil {
		return nil, fmt.Errorf("getting instance ID, %w", err)
	}
	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With("id", id))
	instance, err := c.instanceProvider.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting instance, %w", err)
	}
	instanceType, err := c.resolveInstanceTypeFromInstance(ctx, instance)
	if err != nil {
		return nil, fmt.Errorf("resolving instance type, %w", err)
	}
	return c.instanceToMachine(ctx, instance, instanceType), nil
}

func (c *CloudProvider) LivenessProbe(req *http.Request) error {
	return c.instanceTypeProvider.LivenessProbe(req)
}

// GetInstanceTypes returns all available InstanceTypes
func (c *CloudProvider) GetInstanceTypes(ctx context.Context, provisioner *v1alpha5.Provisioner) ([]*cloudprovider.InstanceType, error) {
	var rawProvider []byte
	provisioner = staticprovisioner.Sp
	if provisioner.Spec.ProviderRef == nil {
		return nil, nil
	}
	if provisioner.Spec.Provider != nil {
		rawProvider = provisioner.Spec.Provider.Raw
	}
	nodeTemplate, err := c.resolveNodeTemplate(ctx, rawProvider, provisioner.Spec.ProviderRef)
	if err != nil {
		return nil, err
	}
	// TODO, break this coupling
	instanceTypes, err := c.instanceTypeProvider.List(ctx, provisioner.Spec.KubeletConfiguration, nodeTemplate)
	if err != nil {
		return nil, err
	}
	return instanceTypes, nil
}

func (c *CloudProvider) Delete(ctx context.Context, machine *v1alpha5.Machine) error {
	return c.instanceProvider.Delete(ctx, machine)
}

func (c *CloudProvider) IsMachineDrifted(ctx context.Context, machine *v1alpha5.Machine) (bool, error) {
	// Not needed when GetInstanceTypes removes provisioner dependency
	//provisioner := &v1alpha5.Provisioner{}
	//if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: machine.Labels[v1alpha5.ProvisionerNameLabelKey]}, provisioner); err != nil {
	//	return false, client.IgnoreNotFound(fmt.Errorf("getting provisioner, %w", err))
	//}
	provisioner := staticprovisioner.Sp
	if provisioner.Spec.ProviderRef == nil {
		return false, nil
	}
	nodeTemplate, err := c.resolveNodeTemplate(ctx, nil, provisioner.Spec.ProviderRef)
	if err != nil {
		return false, client.IgnoreNotFound(fmt.Errorf("resolving node template, %w", err))
	}
	imageDrifted, err := c.isImageDrifted(ctx, machine, provisioner, nodeTemplate)
	if err != nil {
		return false, err
	}
	return imageDrifted, nil
}

// Name returns the CloudProvider implementation name.
func (c *CloudProvider) Name() string {
	return "azure"
}

// TODO: remove nolint on unparam. Added for now in order to pass "make verify" in azure/poc
// nolint: unparam
func (c *CloudProvider) isImageDrifted(
	ctx context.Context, machine *v1alpha5.Machine, provisioner *v1alpha5.Provisioner, _ *v1alpha1.NodeTemplate) (bool, error) {
	instanceTypes, err := c.GetInstanceTypes(ctx, provisioner)
	if err != nil {
		return false, fmt.Errorf("getting instanceTypes, %w", err)
	}
	_, found := lo.Find(instanceTypes, func(instType *cloudprovider.InstanceType) bool {
		return instType.Name == machine.Labels[v1.LabelInstanceTypeStable]
	})
	if !found {
		return false, fmt.Errorf(`finding node instance type "%s"`, machine.Labels[v1.LabelInstanceTypeStable])
	}

	// TODO: implement proper image drift detection, by fetching the VM and checking its imageid vs those in nodeTemplate
	return false, nil
}

func (c *CloudProvider) resolveNodeTemplate(ctx context.Context, raw []byte, objRef *v1alpha5.MachineTemplateRef) (*v1alpha1.NodeTemplate, error) {
	nodeTemplate := &v1alpha1.NodeTemplate{}
	if objRef != nil {
		if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: objRef.Name}, nodeTemplate); err != nil {
			return nil, fmt.Errorf("getting providerRef, %w", err)
		}
		return nodeTemplate, nil
	}
	azure, err := v1alpha1.DeserializeProvider(raw)
	if err != nil {
		return nil, err
	}
	nodeTemplate.Spec.Azure = lo.FromPtr(azure)
	return nodeTemplate, nil
}

func (c *CloudProvider) resolveInstanceTypes(ctx context.Context, machine *v1alpha5.Machine) ([]*cloudprovider.InstanceType, error) {
	//provisionerName, ok := machine.Labels[v1alpha5.ProvisionerNameLabelKey]
	//if !ok {
	//	return nil, fmt.Errorf("finding provisioner owner")
	//}
	//provisioner := &v1alpha5.Provisioner{}
	//if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: provisionerName}, provisioner); err != nil {
	//	return nil, fmt.Errorf("getting provisioner owner, %w", err)
	//}
	provisioner := staticprovisioner.Sp
	instanceTypes, err := c.GetInstanceTypes(ctx, provisioner)
	if err != nil {
		return nil, fmt.Errorf("getting instance types, %w", err)
	}
	reqs := scheduling.NewNodeSelectorRequirements(machine.Spec.Requirements...)
	return lo.Filter(instanceTypes, func(i *cloudprovider.InstanceType, _ int) bool {
		return reqs.Get(v1.LabelInstanceTypeStable).Has(i.Name) && len(i.Offerings.Requirements(reqs).Available()) > 0
	}), nil
}

func (c *CloudProvider) resolveInstanceTypeFromInstance(ctx context.Context, instance *armcompute.VirtualMachine) (*cloudprovider.InstanceType, error) {
	provisioner, err := c.resolveProvisionerFromInstance(ctx, instance)
	if err != nil {
		// If we can't resolve the provisioner, we fallback to not getting instance type info
		return nil, client.IgnoreNotFound(fmt.Errorf("resolving provisioner, %w", err))
	}
	instanceTypes, err := c.GetInstanceTypes(ctx, provisioner)
	if err != nil {
		// If we can't resolve the provisioner, we fallback to not getting instance type info
		return nil, client.IgnoreNotFound(fmt.Errorf("resolving node template, %w", err))
	}
	instanceType, _ := lo.Find(instanceTypes, func(i *cloudprovider.InstanceType) bool {
		return i.Name == string(*instance.Properties.HardwareProfile.VMSize)
	})
	return instanceType, nil
}

func (c *CloudProvider) resolveProvisionerFromInstance(ctx context.Context, instance *armcompute.VirtualMachine) (*v1alpha5.Provisioner, error) {
	//	provisioner := &v1alpha5.Provisioner{}
	//	provisionerName, ok := instance.Tags[v1alpha5.ProvisionerNameLabelKey]
	//	if !ok {
	//		return nil, errors.NewNotFound(schema.GroupResource{Group: v1alpha5.Group, Resource: "Provisioner"}, "")
	//	}
	//	if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: *provisionerName}, provisioner); err != nil {
	//		return nil, err
	//	}
	provisioner := staticprovisioner.Sp
	return provisioner, nil
}

// TODO: revisit; including the differences between labels created here and those passed to kubelet ...
func (c *CloudProvider) instanceToMachine(ctx context.Context, vm *armcompute.VirtualMachine, instanceType *cloudprovider.InstanceType) *v1alpha5.Machine {
	machine := &v1alpha5.Machine{}
	labels := map[string]string{}

	if instanceType != nil {
		labels = getAllSingleValuedRequirementLabels(instanceType)
		machine.Status.Capacity = functional.FilterMap(instanceType.Capacity, func(_ v1.ResourceName, v resource.Quantity) bool { return !resources.IsZero(v) })
		machine.Status.Allocatable = functional.FilterMap(instanceType.Allocatable(), func(_ v1.ResourceName, v resource.Quantity) bool { return !resources.IsZero(v) })
	}
	// TODO: image id? AWS provider:
	// labels[v1alpha1.LabelInstanceAMIID] = aws.StringValue(instance.ImageId)

	if zoneID, err := instance.GetZoneID(vm); err != nil {
		logging.FromContext(ctx).Warnf("Failed to get zone for VM %s, %v", *vm.Name, err)
	} else {
		zone := makeZone(*vm.Location, zoneID)
		// aks-node-validating-webhook protects v1.LabelTopologyZone, will be set elsewhere, so we use a different label
		labels[v1alpha1.AlternativeLabelTopologyZone] = zone
	}

	labels[v1alpha5.LabelCapacityType] = instance.GetCapacityType(vm)
	labels[v1alpha1.LabelSKUHyperVGeneration] = instance.GetHyperVGeneration(vm)

	// TODO: hydrate labels from instance tags
	/*
		if tag, ok := instance.Tags[v1alpha5.ProvisionerNameLabelKey]; ok {
			labels[v1alpha5.ProvisionerNameLabelKey] = *tag
		}
		if tag, ok := instance.Tags[v1alpha5.ManagedByLabelKey]; ok {
			labels[v1alpha5.ManagedByLabelKey] = tag
		}
	*/

	machine.Name = *vm.Name
	machine.Labels = labels
	machine.CreationTimestamp = metav1.Time{Time: *vm.Properties.TimeCreated}

	providerID := fmt.Sprintf("azure://%s", lo.FromPtr(vm.ID))
	// for historical reasons Azure providerID has the resource group name in lower case
	if providerIDLowerRG, err := provider.ConvertResourceGroupNameToLower(providerID); err == nil {
		machine.Status.ProviderID = providerIDLowerRG
	} else {
		logging.FromContext(ctx).Warnf("Failed to convert resource group name to lower case in providerID %s: %v", providerID, err)
		// fallback to original providerID
		machine.Status.ProviderID = providerID
	}

	return machine
}

// TODO: remove duplication here and in the instance package
// getAllSingleValuedRequirementLabels converts instanceType.Requirements to labels
// Like   instanceType.Requirements.Labels() it uses single-valued requirements
// Unlike instanceType.Requirements.Labels() it does not filter out restricted Node labels
func getAllSingleValuedRequirementLabels(instanceType *cloudprovider.InstanceType) map[string]string {
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

// makeZone returns the zone value in format of <region>-<zone-id>.
func makeZone(location string, zoneID string) string {
	return fmt.Sprintf("%s-%s", strings.ToLower(location), zoneID)
}
