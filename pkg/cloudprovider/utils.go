package cloudprovider

import (
	"context"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/utils/functional"
	"github.com/aws/karpenter-core/pkg/utils/resources"
	"github.com/gpu-vmprovisioner/pkg/providers/instance"
	"github.com/gpu-vmprovisioner/pkg/utils"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"knative.dev/pkg/logging"
)

func (c *CloudProvider) instanceToMachine(ctx context.Context, instanceObj *instance.Instance, instanceType *cloudprovider.InstanceType) *v1alpha5.Machine {
	machine := &v1alpha5.Machine{}
	labels := instanceObj.Labels
	annotations := map[string]string{}

	if instanceType != nil {
		labels = lo.Assign(labels, utils.GetAllSingleValuedRequirementLabels(instanceType))
		machine.Status.Capacity = functional.FilterMap(instanceType.Capacity, func(_ v1.ResourceName, v resource.Quantity) bool { return !resources.IsZero(v) })
		machine.Status.Allocatable = functional.FilterMap(instanceType.Allocatable(), func(_ v1.ResourceName, v resource.Quantity) bool { return !resources.IsZero(v) })
	}

	if instanceObj.CapacityType != nil {
		labels[v1alpha5.LabelCapacityType] = *instanceObj.CapacityType
	}

	if v, ok := instanceObj.Tags[v1alpha5.ProvisionerNameLabelKey]; ok {
		labels[v1alpha5.ProvisionerNameLabelKey] = *v
	}
	if v, ok := instanceObj.Tags[v1alpha5.MachineManagedByAnnotationKey]; ok {
		annotations[v1alpha5.MachineManagedByAnnotationKey] = *v
	}

	labels[instance.LabelAgentPoolName] = lo.FromPtr(instanceObj.Name)

	machine.Labels = labels
	machine.Annotations = annotations

	if instanceObj != nil && instanceObj.ID != nil {
		machine.Status.ProviderID = lo.FromPtr(instanceObj.ID)
		annotations[v1alpha5.MachineLinkedAnnotationKey] = lo.FromPtr(instanceObj.ID)
	} else {
		logging.FromContext(ctx).Warnf("Provider ID cannot be nil")
	}

	return machine
}
