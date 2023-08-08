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

package deprovisioning

import (
	"context"
	"errors"
	"fmt"

	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/samber/lo"

	"github.com/aws/karpenter-core/pkg/apis/settings"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/controllers/provisioning"
	"github.com/aws/karpenter-core/pkg/controllers/state"
	"github.com/aws/karpenter-core/pkg/events"
	"github.com/aws/karpenter-core/pkg/metrics"
)

// Drift is a subreconciler that deletes drifted machines.
type Drift struct {
	kubeClient  client.Client
	cluster     *state.Cluster
	provisioner *provisioning.Provisioner
	recorder    events.Recorder
}

func NewDrift(kubeClient client.Client, cluster *state.Cluster, provisioner *provisioning.Provisioner, recorder events.Recorder) *Drift {
	return &Drift{
		kubeClient:  kubeClient,
		cluster:     cluster,
		provisioner: provisioner,
		recorder:    recorder,
	}
}

// ShouldDeprovision is a predicate used to filter deprovisionable machines
func (d *Drift) ShouldDeprovision(ctx context.Context, c *Candidate) bool {
	return settings.FromContext(ctx).DriftEnabled &&
		c.Machine.StatusConditions().GetCondition(v1alpha5.MachineDrifted) != nil &&
		c.Machine.StatusConditions().GetCondition(v1alpha5.MachineDrifted).IsTrue()
}

// ComputeCommand generates a deprovisioning command given deprovisionable machines
func (d *Drift) ComputeCommand(ctx context.Context, nodes ...*Candidate) (Command, error) {
	candidates, err := filterCandidates(ctx, d.kubeClient, d.recorder, nodes)
	if err != nil {
		return Command{}, fmt.Errorf("filtering candidates, %w", err)
	}
	deprovisioningEligibleMachinesGauge.WithLabelValues(d.String()).Set(float64(len(candidates)))

	// Deprovision all empty drifted nodes, as they require no scheduling simulations.
	if empty := lo.Filter(candidates, func(c *Candidate, _ int) bool {
		return len(c.pods) == 0
	}); len(empty) > 0 {
		return Command{
			candidates: empty,
		}, nil
	}

	for _, candidate := range candidates {
		// Check if we need to create any machines.
		results, err := simulateScheduling(ctx, d.kubeClient, d.cluster, d.provisioner, candidate)
		if err != nil {
			// if a candidate machine is now deleting, just retry
			if errors.Is(err, errCandidateDeleting) {
				continue
			}
			return Command{}, err
		}
		// Log when all pods can't schedule, as the command will get executed immediately.
		if !results.AllPodsScheduled() {
			logging.FromContext(ctx).With("machine", candidate.Machine.Name, "node", candidate.Node.Name).Debug("Continuing to terminate drifted machine after scheduling simulation failed to schedule all pods %s", results.PodSchedulingErrors())
		}
		if len(results.NewMachines) == 0 {
			return Command{
				candidates: []*Candidate{candidate},
			}, nil
		}
		return Command{
			candidates:   []*Candidate{candidate},
			replacements: results.NewMachines,
		}, nil
	}
	return Command{}, nil
}

// String is the string representation of the deprovisioner
func (d *Drift) String() string {
	return metrics.DriftReason
}
