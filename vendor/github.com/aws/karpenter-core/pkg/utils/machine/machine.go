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

package machine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/clock"
	"knative.dev/pkg/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/scheduling"
)

// EventHandler is a watcher on v1alpha5.Machine that maps Machines to Nodes based on provider ids
// and enqueues reconcile.Requests for the Nodes
func EventHandler(ctx context.Context, c client.Client) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
		machine := o.(*v1alpha5.Machine)
		nodeList := &v1.NodeList{}
		if machine.Status.ProviderID == "" {
			return nil
		}
		if err := c.List(ctx, nodeList, client.MatchingFields{"spec.providerID": machine.Status.ProviderID}); err != nil {
			return nil
		}
		return lo.Map(nodeList.Items, func(n v1.Node, _ int) reconcile.Request {
			return reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&n),
			}
		})
	})
}

// PodEventHandler is a watcher on v1.Pods that maps Pods to Machine based on the node names
// and enqueues reconcile.Requests for the Machines
func PodEventHandler(ctx context.Context, c client.Client) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(o client.Object) (requests []reconcile.Request) {
		if name := o.(*v1.Pod).Spec.NodeName; name != "" {
			node := &v1.Node{}
			if err := c.Get(ctx, types.NamespacedName{Name: name}, node); err != nil {
				return []reconcile.Request{}
			}
			machineList := &v1alpha5.MachineList{}
			if err := c.List(ctx, machineList, client.MatchingFields{"status.providerID": node.Spec.ProviderID}); err != nil {
				return []reconcile.Request{}
			}
			return lo.Map(machineList.Items, func(m v1alpha5.Machine, _ int) reconcile.Request {
				return reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(&m),
				}
			})
		}
		return requests
	})
}

// NodeEventHandler is a watcher on v1.Node that maps Nodes to Machines based on provider ids
// and enqueues reconcile.Requests for the Machines
func NodeEventHandler(ctx context.Context, c client.Client) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
		node := o.(*v1.Node)
		machineList := &v1alpha5.MachineList{}
		if err := c.List(ctx, machineList, client.MatchingFields{"status.providerID": node.Spec.ProviderID}); err != nil {
			return []reconcile.Request{}
		}
		return lo.Map(machineList.Items, func(m v1alpha5.Machine, _ int) reconcile.Request {
			return reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&m),
			}
		})
	})
}

// ProvisionerEventHandler is a watcher on v1alpha5.Machine that maps Provisioner to Machines based
// on the v1alpha5.ProvsionerNameLabelKey and enqueues reconcile.Requests for the Machine
func ProvisionerEventHandler(ctx context.Context, c client.Client) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(o client.Object) (requests []reconcile.Request) {
		machineList := &v1alpha5.MachineList{}
		if err := c.List(ctx, machineList, client.MatchingLabels(map[string]string{v1alpha5.ProvisionerNameLabelKey: o.GetName()})); err != nil {
			return requests
		}
		return lo.Map(machineList.Items, func(machine v1alpha5.Machine, _ int) reconcile.Request {
			return reconcile.Request{NamespacedName: types.NamespacedName{Name: machine.Name}}
		})
	})
}

// NodeNotFoundError is an error returned when no v1.Nodes are found matching the passed providerID
type NodeNotFoundError struct {
	ProviderID string
}

func (e *NodeNotFoundError) Error() string {
	return fmt.Sprintf("no nodes found for provider id '%s'", e.ProviderID)
}

func IsNodeNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	nnfErr := &NodeNotFoundError{}
	return errors.As(err, &nnfErr)
}

func IgnoreNodeNotFoundError(err error) error {
	if !IsNodeNotFoundError(err) {
		return err
	}
	return nil
}

// DuplicateNodeError is an error returned when multiple v1.Nodes are found matching the passed providerID
type DuplicateNodeError struct {
	ProviderID string
}

func (e *DuplicateNodeError) Error() string {
	return fmt.Sprintf("multiple found for provider id '%s'", e.ProviderID)
}

func IsDuplicateNodeError(err error) bool {
	if err == nil {
		return false
	}
	dnErr := &DuplicateNodeError{}
	return errors.As(err, &dnErr)
}

func IgnoreDuplicateNodeError(err error) error {
	if !IsDuplicateNodeError(err) {
		return err
	}
	return nil
}

// NodeForMachine is a helper function that takes a v1alpha5.Machine and attempts to find the matching v1.Node by its providerID
// This function will return errors if:
//  1. No v1.Nodes match the v1alpha5.Machine providerID
//  2. Multiple v1.Nodes match the v1alpha5.Machine providerID
func NodeForMachine(ctx context.Context, c client.Client, machine *v1alpha5.Machine) (*v1.Node, error) {
	nodes, err := AllNodesForMachine(ctx, c, machine)
	if err != nil {
		return nil, err
	}
	// If the providerID is defined, use that value; else, use the machine linked annotation if it's on the machine
	providerID := lo.Ternary(machine.Status.ProviderID != "", machine.Status.ProviderID, machine.Annotations[v1alpha5.MachineLinkedAnnotationKey])
	if len(nodes) > 1 {
		return nil, &DuplicateNodeError{ProviderID: providerID}
	}
	if len(nodes) == 0 {
		return nil, &NodeNotFoundError{ProviderID: providerID}
	}
	return nodes[0], nil
}

// AllNodesForMachine is a helper function that takes a v1alpha5.Machine and finds ALL matching v1.Nodes by their providerID
// If the providerID is not resolved for a Machine, then no Nodes will map to it
func AllNodesForMachine(ctx context.Context, c client.Client, machine *v1alpha5.Machine) ([]*v1.Node, error) {
	// If the providerID is defined, use that value; else, use the machine linked annotation if it's on the machine
	providerID := lo.Ternary(machine.Status.ProviderID != "", machine.Status.ProviderID, machine.Annotations[v1alpha5.MachineLinkedAnnotationKey])
	// Machines that have no resolved providerID have no nodes mapped to them
	if providerID == "" {
		return nil, nil
	}
	nodeList := v1.NodeList{}
	if err := c.List(ctx, &nodeList, client.MatchingFields{"spec.providerID": providerID}); err != nil {
		return nil, fmt.Errorf("listing nodes, %w", err)
	}
	return lo.ToSlicePtr(nodeList.Items), nil
}

// New converts a node into a Machine using known values from the node and provisioner spec values
// Deprecated: This Machine generator function can be removed when v1beta1 migration has completed.
func New(node *v1.Node, provisioner *v1alpha5.Provisioner) *v1alpha5.Machine {
	machine := NewFromNode(node)
	machine.Annotations = lo.Assign(provisioner.Annotations, v1alpha5.ProviderAnnotation(provisioner.Spec.Provider))
	machine.Labels = lo.Assign(provisioner.Labels, map[string]string{v1alpha5.ProvisionerNameLabelKey: provisioner.Name})
	machine.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion:         v1alpha5.SchemeGroupVersion.String(),
			Kind:               "Provisioner",
			Name:               provisioner.Name,
			UID:                provisioner.UID,
			BlockOwnerDeletion: ptr.Bool(true),
		},
	}
	machine.Spec.Kubelet = provisioner.Spec.KubeletConfiguration
	machine.Spec.Taints = provisioner.Spec.Taints
	machine.Spec.StartupTaints = provisioner.Spec.StartupTaints
	machine.Spec.Requirements = provisioner.Spec.Requirements
	machine.Spec.MachineTemplateRef = provisioner.Spec.ProviderRef
	return machine
}

// NewFromNode converts a node into a pseudo-Machine using known values from the node
// Deprecated: This Machine generator function can be removed when v1beta1 migration has completed.
func NewFromNode(node *v1.Node) *v1alpha5.Machine {
	m := &v1alpha5.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        node.Name,
			Annotations: node.Annotations,
			Labels:      node.Labels,
			Finalizers:  []string{v1alpha5.TerminationFinalizer},
		},
		Spec: v1alpha5.MachineSpec{
			Taints:       node.Spec.Taints,
			Requirements: scheduling.NewLabelRequirements(node.Labels).NodeSelectorRequirements(),
			Resources: v1alpha5.ResourceRequirements{
				Requests: node.Status.Allocatable,
			},
		},
		Status: v1alpha5.MachineStatus{
			ProviderID:  node.Spec.ProviderID,
			Capacity:    node.Status.Capacity,
			Allocatable: node.Status.Allocatable,
		},
	}
	if _, ok := node.Labels[v1alpha5.LabelNodeInitialized]; ok {
		m.StatusConditions().MarkTrue(v1alpha5.MachineInitialized)
	}
	m.StatusConditions().MarkTrue(v1alpha5.MachineLaunched)
	m.StatusConditions().MarkTrue(v1alpha5.MachineRegistered)
	return m
}

func IsPastEmptinessTTL(machine *v1alpha5.Machine, clock clock.Clock, provisioner *v1alpha5.Provisioner) bool {
	return machine.StatusConditions().GetCondition(v1alpha5.MachineEmpty) != nil &&
		machine.StatusConditions().GetCondition(v1alpha5.MachineEmpty).IsTrue() &&
		!clock.Now().Before(machine.StatusConditions().GetCondition(v1alpha5.MachineEmpty).LastTransitionTime.Inner.Add(time.Duration(lo.FromPtr(provisioner.Spec.TTLSecondsAfterEmpty))*time.Second))
}

func IsExpired(obj client.Object, clock clock.Clock, provisioner *v1alpha5.Provisioner) bool {
	return clock.Now().After(GetExpirationTime(obj, provisioner))
}

func GetExpirationTime(obj client.Object, provisioner *v1alpha5.Provisioner) time.Time {
	if provisioner == nil || provisioner.Spec.TTLSecondsUntilExpired == nil || obj == nil {
		// If not defined, return some much larger time.
		return time.Date(5000, 0, 0, 0, 0, 0, 0, time.UTC)
	}
	expirationTTL := time.Duration(ptr.Int64Value(provisioner.Spec.TTLSecondsUntilExpired)) * time.Second
	return obj.GetCreationTimestamp().Add(expirationTTL)
}
