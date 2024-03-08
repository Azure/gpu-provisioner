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
