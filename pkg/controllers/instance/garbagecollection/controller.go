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

package garbagecollection

import (
	"context"
	"time"

	"github.com/awslabs/operatorpkg/singleton"
	"github.com/samber/lo"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"knative.dev/pkg/logging"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/operator/injection"
	nodeclaimutil "sigs.k8s.io/karpenter/pkg/utils/nodeclaim"
)

type Controller struct {
	kubeClient    client.Client
	cloudProvider cloudprovider.CloudProvider
}

func NewController(kubeClient client.Client, cloudProvider cloudprovider.CloudProvider) *Controller {
	return &Controller{
		kubeClient:    kubeClient,
		cloudProvider: cloudProvider,
	}
}

func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	ctx = injection.WithControllerName(ctx, "instance.garbagecollection")
	// list all agentpools
	cloudNodeClaims, err := c.cloudProvider.List(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	cloudNodeClaims = lo.Filter(cloudNodeClaims, func(nc *v1.NodeClaim, _ int) bool {
		return nc.DeletionTimestamp.IsZero()
	})

	kaitoNodeClaims, err := nodeclaimutil.AllKaitoNodeClaims(ctx, c.kubeClient)
	if err != nil {
		return reconcile.Result{}, err
	}

	clusterNodeClaimNames := sets.New[string](lo.FilterMap(kaitoNodeClaims, func(nc v1.NodeClaim, _ int) (string, bool) {
		return nc.Name, true
	})...)

	// instance's related NodeClaim has been removed, and instance has been created for more than 10min
	// so we need to garbage these leaked cloudprovider instances and nodes.
	deletedCloudProviderInstances := lo.Filter(cloudNodeClaims, func(nc *v1.NodeClaim, _ int) bool {
		if clusterNodeClaimNames.Has(nc.Name) {
			return false
		}

		if !nc.CreationTimestamp.IsZero() {
			// agentpool has been created less than 30 seconds, skip it
			if nc.CreationTimestamp.Time.Add(30 * time.Second).After(time.Now()) {
				return false
			}
		}

		return true
	})
	logging.FromContext(ctx).Infow("instance garbagecollection status", "garbaged instance count", len(deletedCloudProviderInstances))

	errs := make([]error, len(deletedCloudProviderInstances))
	workqueue.ParallelizeUntil(ctx, 20, len(deletedCloudProviderInstances), func(i int) {
		if err := c.cloudProvider.Delete(ctx, deletedCloudProviderInstances[i]); err != nil {
			logging.FromContext(ctx).Errorf("failed to delete leaked cloudprovider instance(%s), %v", deletedCloudProviderInstances[i].Name, err)
			errs[i] = cloudprovider.IgnoreNodeClaimNotFoundError(err)
			return
		}
		logging.FromContext(ctx).Infow("delete leaked cloudprovider instance successfully", "name", deletedCloudProviderInstances[i].Name)

		if len(deletedCloudProviderInstances[i].Status.ProviderID) != 0 {
			nodes, err := nodeclaimutil.AllNodesForNodeClaim(ctx, c.kubeClient, deletedCloudProviderInstances[i])
			if err != nil {
				errs[i] = err
				return
			}

			subErrs := make([]error, len(nodes))
			for k := range nodes {
				// If we still get the Node, but it's already marked as terminating, we don't need to call Delete again
				if nodes[k].DeletionTimestamp.IsZero() {
					// We delete nodes to trigger the node finalization and deletion flow
					if err := c.kubeClient.Delete(ctx, nodes[k]); client.IgnoreNotFound(err) != nil {
						logging.FromContext(ctx).Errorf("failed to delete leaked node(%s), %v", nodes[k].Name, err)
						subErrs[k] = err
					} else {
						logging.FromContext(ctx).Infow("delete leaked node successfully", "name", nodes[k].Name)
					}
				}
			}
			errs[i] = multierr.Combine(subErrs...)
		}
	})

	return reconcile.Result{RequeueAfter: time.Minute * 2}, multierr.Combine(errs...)
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named("instance.garbagecollection").
		WatchesRawSource(singleton.Source()).
		Complete(singleton.AsReconciler(c))
}
