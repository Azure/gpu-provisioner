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

package test

import (
	"fmt"

	"github.com/imdario/mergo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
)

type NodeOptions struct {
	metav1.ObjectMeta
	ReadyStatus   v1.ConditionStatus
	ReadyReason   string
	Conditions    []v1.NodeCondition
	Unschedulable bool
	ProviderID    string
	Taints        []v1.Taint
	Allocatable   v1.ResourceList
	Capacity      v1.ResourceList
}

func Node(overrides ...NodeOptions) *v1.Node {
	options := NodeOptions{}
	for _, opts := range overrides {
		if err := mergo.Merge(&options, opts, mergo.WithOverride); err != nil {
			panic(fmt.Sprintf("Failed to merge node options: %s", err))
		}
	}
	if options.ReadyStatus == "" {
		options.ReadyStatus = v1.ConditionTrue
	}
	if options.Capacity == nil {
		options.Capacity = options.Allocatable
	}

	return &v1.Node{
		ObjectMeta: ObjectMeta(options.ObjectMeta),
		Spec: v1.NodeSpec{
			Unschedulable: options.Unschedulable,
			Taints:        options.Taints,
			ProviderID:    options.ProviderID,
		},
		Status: v1.NodeStatus{
			Allocatable: options.Allocatable,
			Capacity:    options.Capacity,
			Conditions:  []v1.NodeCondition{{Type: v1.NodeReady, Status: options.ReadyStatus, Reason: options.ReadyReason}},
		},
	}
}

func MachineLinkedNode(machine *v1alpha5.Machine) *v1.Node {
	return Node(
		NodeOptions{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      machine.Labels,
				Annotations: machine.Annotations,
				Finalizers:  machine.Finalizers,
			},
			Taints:      append(machine.Spec.Taints, machine.Spec.StartupTaints...),
			Capacity:    machine.Status.Capacity,
			Allocatable: machine.Status.Allocatable,
			ProviderID:  machine.Status.ProviderID,
		},
	)
}
