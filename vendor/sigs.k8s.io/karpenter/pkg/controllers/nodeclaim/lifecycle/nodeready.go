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

package lifecycle

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	nodeclaimutil "sigs.k8s.io/karpenter/pkg/utils/nodeclaim"
)

type NodeReady struct {
	kubeClient client.Client
}

func (r *NodeReady) Reconcile(ctx context.Context, nodeClaim *v1.NodeClaim) (reconcile.Result, error) {
	if !nodeClaim.StatusConditions().Get(v1.ConditionTypeInitialized).IsTrue() {
		nodeClaim.StatusConditions().SetUnknownWithReason(v1.ConditionTypeNodeReady, "NodeClaimNotInitialized", "node claim is not initialized")
		return reconcile.Result{}, nil
	}

	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues("provider-id", nodeClaim.Status.ProviderID))
	node, err := nodeclaimutil.NodeForNodeClaim(ctx, r.kubeClient, nodeClaim)
	if err != nil {
		if nodeclaimutil.IsNodeNotFoundError(err) {
			nodeClaim.StatusConditions().SetUnknownWithReason(v1.ConditionTypeNodeReady, "NodeNotFound", "Node not registered with cluster")
			return reconcile.Result{}, nil
		}
		if nodeclaimutil.IsDuplicateNodeError(err) {
			nodeClaim.StatusConditions().SetFalse(v1.ConditionTypeNodeReady, "MultipleNodesFound", "Invariant violated, matched multiple nodes")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("getting node for nodeclaim, %w", err)
	}

	if isNodeReady(node) {
		nodeClaim.StatusConditions().SetTrue(v1.ConditionTypeNodeReady)
	} else {
		nodeClaim.StatusConditions().SetFalse(v1.ConditionTypeNodeReady, "NodeNotReady", "Node status is NotReady")
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
