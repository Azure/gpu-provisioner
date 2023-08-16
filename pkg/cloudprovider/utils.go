package cloudprovider

import (
	"context"
	"fmt"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/utils/functional"
	"github.com/aws/karpenter-core/pkg/utils/resources"
	"github.com/gpu-vmprovisioner/pkg/providers/instance"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/logging"
)

func (c *CloudProvider) instanceToMachine(ctx context.Context, instance *instance.Instance, instanceType *cloudprovider.InstanceType) *v1alpha5.Machine {
	machine := &v1alpha5.Machine{}
	labels := map[string]string{}
	annotations := map[string]string{}

	if instanceType != nil {
		labels = getAllSingleValuedRequirementLabels(instanceType)
		machine.Status.Capacity = functional.FilterMap(instanceType.Capacity, func(_ v1.ResourceName, v resource.Quantity) bool { return !resources.IsZero(v) })
		machine.Status.Allocatable = functional.FilterMap(instanceType.Allocatable(), func(_ v1.ResourceName, v resource.Quantity) bool { return !resources.IsZero(v) })
	}

	if instance.CapacityType != nil {
		labels[v1alpha5.LabelCapacityType] = *instance.CapacityType
	}

	if v, ok := instance.Tags[v1alpha5.ProvisionerNameLabelKey]; ok {
		labels[v1alpha5.ProvisionerNameLabelKey] = *v
	}
	if v, ok := instance.Tags[v1alpha5.MachineManagedByAnnotationKey]; ok {
		annotations[v1alpha5.MachineManagedByAnnotationKey] = *v
	}

	machine.Labels = labels
	machine.Annotations = annotations

	machine.CreationTimestamp = metav1.Time{Time: instance.LaunchTime}

	if instance != nil && instance.ID != nil {
		machine.Status.ProviderID = fmt.Sprintf("azure://%s", lo.FromPtr(instance.ID))
	} else {
		logging.FromContext(ctx).Warnf("Provider ID cannot be nil")
	}

	return machine
}

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
