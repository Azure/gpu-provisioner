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

package consistency

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/clock"
	"knative.dev/pkg/logging"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/events"
	corecontroller "github.com/aws/karpenter-core/pkg/operator/controller"
	machineutil "github.com/aws/karpenter-core/pkg/utils/machine"
)

var _ corecontroller.TypedController[*v1alpha5.Machine] = (*Controller)(nil)

type Controller struct {
	clock       clock.Clock
	kubeClient  client.Client
	checks      []Check
	recorder    events.Recorder
	lastScanned *cache.Cache
}

type Issue string

type Check interface {
	// Check performs the consistency check, this should return a list of slice discovered, or an empty
	// slice if no issues were found
	Check(context.Context, *v1.Node, *v1alpha5.Machine) ([]Issue, error)
}

// scanPeriod is how often we inspect and report issues that are found.
const scanPeriod = 10 * time.Minute

func NewController(clk clock.Clock, kubeClient client.Client, recorder events.Recorder,
	provider cloudprovider.CloudProvider) corecontroller.Controller {

	return corecontroller.Typed[*v1alpha5.Machine](kubeClient, &Controller{
		clock:       clk,
		kubeClient:  kubeClient,
		recorder:    recorder,
		lastScanned: cache.New(scanPeriod, 1*time.Minute),
		checks: []Check{
			NewTermination(kubeClient),
			NewNodeShape(provider),
		}},
	)
}

func (c *Controller) Name() string {
	return "consistency"
}

func (c *Controller) Reconcile(ctx context.Context, machine *v1alpha5.Machine) (reconcile.Result, error) {
	if machine.Status.ProviderID == "" {
		return reconcile.Result{}, nil
	}
	// If we get an event before we should check for consistency checks, we ignore and wait
	if lastTime, ok := c.lastScanned.Get(client.ObjectKeyFromObject(machine).String()); ok {
		if lastTime, ok := lastTime.(time.Time); ok {
			remaining := scanPeriod - c.clock.Since(lastTime)
			return reconcile.Result{RequeueAfter: remaining}, nil
		}
		// the above should always succeed
		return reconcile.Result{RequeueAfter: scanPeriod}, nil
	}
	c.lastScanned.SetDefault(client.ObjectKeyFromObject(machine).String(), c.clock.Now())

	// We assume the invariant that there is a single node for a single machine. If this invariant is violated,
	// then we assume this is bubbled up through the machine lifecycle controller and don't perform consistency checks
	node, err := machineutil.NodeForMachine(ctx, c.kubeClient, machine)
	if err != nil {
		return reconcile.Result{}, machineutil.IgnoreDuplicateNodeError(machineutil.IgnoreNodeNotFoundError(err))
	}
	for _, check := range c.checks {
		issues, err := check.Check(ctx, node, machine)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("checking node with %T, %w", check, err)
		}
		for _, issue := range issues {
			logging.FromContext(ctx).Errorf("check failed, %s", issue)
			consistencyErrors.With(prometheus.Labels{checkLabel: reflect.TypeOf(check).Elem().Name()}).Inc()
			c.recorder.Publish(CheckEvent(machine, string(issue)))
		}
	}
	return reconcile.Result{RequeueAfter: scanPeriod}, nil
}

func (c *Controller) Builder(ctx context.Context, m manager.Manager) corecontroller.Builder {
	return corecontroller.Adapt(controllerruntime.
		NewControllerManagedBy(m).
		For(&v1alpha5.Machine{}).
		Watches(
			&source.Kind{Type: &v1.Node{}},
			machineutil.NodeEventHandler(ctx, c.kubeClient),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}),
	)
}
