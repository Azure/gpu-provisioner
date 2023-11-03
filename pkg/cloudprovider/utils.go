package cloudprovider

import (
	"context"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/azure/gpu-provisioner/pkg/providers/instance"
	"github.com/samber/lo"
	"knative.dev/pkg/logging"
)

func (c *CloudProvider) instanceToMachine(ctx context.Context, instanceObj *instance.Instance) *v1alpha5.Machine {
	machine := &v1alpha5.Machine{}
	labels := instanceObj.Labels
	annotations := map[string]string{}

	machine.Name = lo.FromPtr(instanceObj.Name)

	if instanceObj.CapacityType != nil {
		labels[v1alpha5.LabelCapacityType] = *instanceObj.CapacityType
	}

	if v, ok := instanceObj.Tags[v1alpha5.ProvisionerNameLabelKey]; ok {
		labels[v1alpha5.ProvisionerNameLabelKey] = *v
	}
	if v, ok := instanceObj.Tags[v1alpha5.MachineManagedByAnnotationKey]; ok {
		annotations[v1alpha5.MachineManagedByAnnotationKey] = *v
	}

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
