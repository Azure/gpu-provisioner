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

package garbagecollection

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	"go.uber.org/multierr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/metrics"
	corecontroller "github.com/aws/karpenter-core/pkg/operator/controller"
	"github.com/aws/karpenter-core/pkg/utils/sets"
)

const (
	NodeHeartBeatAnnotationKey = "NodeHeartBeatTimeStamp"
)

type Controller struct {
	clock         clock.Clock
	kubeClient    client.Client
	cloudProvider cloudprovider.CloudProvider
}

func NewController(c clock.Clock, kubeClient client.Client, cloudProvider cloudprovider.CloudProvider) corecontroller.Controller {
	return &Controller{
		clock:         c,
		kubeClient:    kubeClient,
		cloudProvider: cloudProvider,
	}
}

func (c *Controller) Name() string {
	return "machine.garbagecollection"
}

func (c *Controller) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	merr := c.reconcileMachines(ctx)
	nerr := c.reconcileNodes(ctx)

	return reconcile.Result{RequeueAfter: time.Minute * 2}, multierr.Combine(merr, nerr)
}

// gpu-provisioner: node name conflict means two machines are linked to the same node.
// AKS APIs only support PUT not POST for creating a new agent pool. The name conflict during
// creation will not return error. The latest create call will update the agent pool unconditionally.
// We cannot completlely avoid the conflict.
// But since we use machine name as agent pool name, the chance of hitting agent pool name conflict is very low.
// We rely on periodic checks to remediate the conflict if any.
func (c *Controller) remediateNodeNameConflict(ctx context.Context) error {
	machineList := &v1alpha5.MachineList{}
	if err := c.kubeClient.List(ctx, machineList, client.HasLabels([]string{"kaito.sh/workspace"})); err != nil {
		return err
	}

	nodeToMachineMap := make(map[string][]*v1alpha5.Machine)

	for i, _ := range machineList.Items {
		if node, linked := machineList.Items[i].Annotations[v1alpha5.MachineLinkedAnnotationKey]; linked {
			if _, ok := nodeToMachineMap[node]; !ok {
				nodeToMachineMap[node] = []*v1alpha5.Machine{&machineList.Items[i]}
			} else {
				nodeToMachineMap[node] = append(nodeToMachineMap[node], &machineList.Items[i])
			}
		}
	}

	for _, v := range nodeToMachineMap {
		// Detect node name conflict, the remediation is to remove extra machines.
		if len(v) > 1 {
			// There is no particular order to decide which ones should be deleted.
			for i := 1; i < len(v); i++ {
				stored := v[i].DeepCopy()
				updated := v[i].DeepCopy()
				updated.Annotations[v1alpha5.MachineLinkedAnnotationKey] = ""
				updated.Status.ProviderID = ""
				controllerutil.RemoveFinalizer(updated, v1alpha5.TerminationFinalizer)

				updatedCopy := updated.DeepCopy()
				if err := c.kubeClient.Patch(ctx, updated, client.MergeFrom(stored)); client.IgnoreNotFound(err) != nil {
					return err
				}
				if err := c.kubeClient.Status().Patch(ctx, updatedCopy, client.MergeFrom(stored)); client.IgnoreNotFound(err) != nil {
					return err
				}
				// After clear the provider id, we are safe to delete the machine.
				if err := c.kubeClient.Delete(ctx, stored); client.IgnoreNotFound(err) != nil {
					return err
				}
				logging.FromContext(ctx).Debugf("delete machine %s because it has a node name conflict with machine %s", v[i].Name, v[0].Name)
			}
		}
	}
	return nil
}

func (c *Controller) reconcileNodes(ctx context.Context) error {
	nodeList := &v1.NodeList{}
	if err := c.kubeClient.List(ctx, nodeList); err != nil {
		return err
	}

	machineList := &v1alpha5.MachineList{}
	if err := c.kubeClient.List(ctx, machineList, client.HasLabels([]string{"kaito.sh/workspace"})); err != nil {
		return err
	}

	machineNames := make(map[string]struct{})
	for _, each := range machineList.Items {
		machineNames[each.Name] = struct{}{}
	}

	deletedGarbageNodes := lo.Filter(lo.ToSlicePtr(nodeList.Items), func(n *v1.Node, _ int) bool {
		_, ok := n.Labels[v1alpha5.ProvisionerNameLabelKey]
		if ok && n.Spec.ProviderID != "" {
			// We rely on a strong assumption here: the machine name is equal to the agent pool name.
			machine := n.Labels["kubernetes.azure.com/agentpool"]
			if _, ok := machineNames[machine]; !ok {
				return true
			}
		}
		return false
	})

	errs := make([]error, len(deletedGarbageNodes))
	workqueue.ParallelizeUntil(ctx, 20, len(deletedGarbageNodes), func(i int) {
		// We delete nodes to trigger the node finalization and deletion flow.
		if err := c.kubeClient.Delete(ctx, deletedGarbageNodes[i]); client.IgnoreNotFound(err) != nil {
			errs[i] = err
			return
		}
		logging.FromContext(ctx).Debugf("garbage collecting node %s since the linked machine does not exist", deletedGarbageNodes[i].Name)
	})
	return multierr.Combine(errs...)
}

// gpu-provisioner: leverage the two minutes perodic check to update machine readiness heartbeat.
func (c *Controller) reconcileMachines(ctx context.Context) error {
	machineList := &v1alpha5.MachineList{}
	if err := c.kubeClient.List(ctx, machineList, client.HasLabels([]string{"kaito.sh/workspace"})); err != nil {
		return err
	}

	// The NotReady nodes are excluded from the list.
	cloudProviderMachines, err := c.cloudProvider.List(ctx)
	if err != nil {
		return err
	}
	cloudProviderMachines = lo.Filter(cloudProviderMachines, func(m *v1alpha5.Machine, _ int) bool {
		return m.DeletionTimestamp.IsZero()
	})
	cloudProviderProviderIDs := sets.New[string](lo.Map(cloudProviderMachines, func(m *v1alpha5.Machine, _ int) string {
		return m.Status.ProviderID
	})...)

	deletedGarbageMachines := lo.Filter(lo.ToSlicePtr(machineList.Items), func(m *v1alpha5.Machine, _ int) bool {
		// The assumption is that any clean up work should be done in 10 minutes.
		// The node gc will cover any problems caused by this force deletion.
		return !m.DeletionTimestamp.IsZero() && metav1.Now().After((*m.DeletionTimestamp).Add(time.Minute*10))
	})

	rfErrs := c.batchDeleteMachines(ctx, deletedGarbageMachines, true, "to be delete but blocked by finializer for more than 10 minutes")

	// Check all machine heartbeats.
	hbMachines := lo.Filter(lo.ToSlicePtr(machineList.Items), func(m *v1alpha5.Machine, _ int) bool {
		return m.StatusConditions().GetCondition(v1alpha5.MachineLaunched).IsTrue() &&
			m.DeletionTimestamp.IsZero()
	})

	var hbUpdated atomic.Uint64
	deletedNotReadyMachines := []*v1alpha5.Machine{}
	// Update machine heartbeat.
	hbErrs := make([]error, len(hbMachines))
	workqueue.ParallelizeUntil(ctx, 20, len(hbMachines), func(i int) {
		stored := hbMachines[i].DeepCopy()
		updated := hbMachines[i].DeepCopy()

		if cloudProviderProviderIDs.Has(stored.Status.ProviderID) {
			hbUpdated.Add(1)
			if updated.Annotations == nil {
				updated.Annotations = make(map[string]string)
			}

			timeStr, _ := metav1.NewTime(time.Now()).MarshalJSON()
			updated.Annotations[NodeHeartBeatAnnotationKey] = string(timeStr)

			// If the machine was not ready, it becomes ready after getting the heartbeat.
			updated.StatusConditions().MarkTrue("Ready")
		} else {
			logging.FromContext(ctx).Debugf(fmt.Sprintf("machine %s does not receive hb", stored.Name))
			updated.StatusConditions().MarkFalse("Ready", "NodeNotReady", "Node status is NotReady")
		}
		statusCopy := updated.DeepCopy()
		updateCopy := updated.DeepCopy()
		if err := c.kubeClient.Patch(ctx, updated, client.MergeFrom(stored)); err != nil {
			hbErrs[i] = client.IgnoreNotFound(err)
			return
		}
		if err := c.kubeClient.Status().Patch(ctx, statusCopy, client.MergeFrom(stored)); err != nil {
			hbErrs[i] = client.IgnoreNotFound(err)
			return
		}
		if !updateCopy.StatusConditions().IsHappy() &&
			c.clock.Since(updateCopy.StatusConditions().GetCondition("Ready").LastTransitionTime.Inner.Time) > time.Minute*10 {
			deletedNotReadyMachines = append(deletedNotReadyMachines, updateCopy)
		}
	})
	if len(hbMachines) > 0 {
		logging.FromContext(ctx).Debugf(fmt.Sprintf("Update heartbeat for %d out of %d machines", hbUpdated.Load(), len(hbMachines)))
	}
	errs := c.batchDeleteMachines(ctx, deletedNotReadyMachines, false, "being NotReady for more than 10 minutes")
	errs = append(errs, hbErrs...)
	errs = append(errs, rfErrs...)
	return multierr.Combine(errs...)
}

func (c *Controller) batchDeleteMachines(ctx context.Context, machines []*v1alpha5.Machine, removeFinalizer bool, msg string) []error {
	errs := make([]error, len(machines))
	workqueue.ParallelizeUntil(ctx, 20, len(machines), func(i int) {
		if removeFinalizer {
			stored := machines[i].DeepCopy()
			updated := machines[i].DeepCopy()
			controllerutil.RemoveFinalizer(updated, v1alpha5.TerminationFinalizer)
			if !equality.Semantic.DeepEqual(stored, updated) {
				if err := c.kubeClient.Patch(ctx, updated, client.MergeFrom(stored)); client.IgnoreNotFound(err) != nil {
					errs[i] = err
					return
				}
			}
		} else if err := c.kubeClient.Delete(ctx, machines[i]); client.IgnoreNotFound(err) != nil {
			errs[i] = err
			return
		}
		logging.FromContext(ctx).
			With("provisioner", machines[i].Labels[v1alpha5.ProvisionerNameLabelKey], "machine", machines[i].Name, "provider-id", machines[i].Status.ProviderID).
			Debugf("garbage collecting machine with reason: %s", msg)
		metrics.MachinesTerminatedCounter.With(prometheus.Labels{
			metrics.ReasonLabel:      "garbage_collected",
			metrics.ProvisionerLabel: machines[i].Labels[v1alpha5.ProvisionerNameLabelKey],
		}).Inc()
	})
	return errs
}

func (c *Controller) Builder(_ context.Context, m manager.Manager) corecontroller.Builder {
	return corecontroller.NewSingletonManagedBy(m)
}
