/*
Copyright The Kubernetes Authors.

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

package nodeclaim

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	nodeclaimutil "sigs.k8s.io/karpenter/pkg/utils/nodeclaim"
)

var (
	nodeSelectorPredicate, _ = predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: v1.NodePoolLabelKey, Operator: metav1.LabelSelectorOpExists},
		},
	})
)

type Controller struct {
	kubeClient client.Client
}

func NewController(kubeClient client.Client) *Controller {
	return &Controller{
		kubeClient: kubeClient,
	}
}

func (c *Controller) Reconcile(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	if !node.GetDeletionTimestamp().IsZero() || len(node.Spec.ProviderID) == 0 {
		return reconcile.Result{}, nil
	}

	// skip node which is not created from nodeclaim
	if _, ok := node.Labels[v1.NodePoolLabelKey]; !ok {
		return reconcile.Result{}, nil
	}

	nodeClaimList := &v1.NodeClaimList{}
	if err := c.kubeClient.List(ctx, nodeClaimList, client.MatchingFields{"status.providerID": node.Spec.ProviderID}); err != nil {
		return reconcile.Result{}, err
	}

	var nodeClaim *v1.NodeClaim
	if len(nodeClaimList.Items) == 0 {
		return reconcile.Result{}, fmt.Errorf("nodeclaim not found for node(%s)", node.Name)
	} else if len(nodeClaimList.Items) > 1 {
		return reconcile.Result{}, fmt.Errorf("more than one nodeclaim found for node(%s)", node.Name)
	} else {
		nodeClaim = &nodeClaimList.Items[0]
	}

	stored := nodeClaim.DeepCopy()
	if !nodeClaim.StatusConditions().Get(v1.ConditionTypeInitialized).IsTrue() {
		nodeClaim.StatusConditions().SetUnknownWithReason(v1.ConditionTypeNodeReady, "NodeClaimNotInitialized", "node claim is not initialized")
	} else {
		if isNodeReady(node) {
			nodeClaim.StatusConditions().SetTrue(v1.ConditionTypeNodeReady)
		} else {
			nodeClaim.StatusConditions().SetFalse(v1.ConditionTypeNodeReady, "NodeNotReady", "Node status is NotReady")
		}
	}

	if !equality.Semantic.DeepEqual(stored, nodeClaim) {
		if err := c.kubeClient.Status().Patch(ctx, nodeClaim, client.MergeFrom(stored)); err != nil {
			return reconcile.Result{}, client.IgnoreNotFound(err)
		}
	}

	return reconcile.Result{}, nil
}

func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}

	return false
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("nodeclaim.status").
		For(&corev1.Node{},
			builder.WithPredicates(
				predicate.Funcs{
					CreateFunc: func(e event.CreateEvent) bool { return false },
					UpdateFunc: func(e event.UpdateEvent) bool { return true },
					DeleteFunc: func(e event.DeleteEvent) bool { return false },
				},
			),
		).
		WithEventFilter(nodeclaimutil.KaitoResourcePredicate).
		WithEventFilter(nodeSelectorPredicate).
		Complete(reconcile.AsReconciler(m.GetClient(), c))
}
