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
	"fmt"

	"k8s.io/utils/clock"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/controllers/provisioning"
	"github.com/aws/karpenter-core/pkg/controllers/state"
	"github.com/aws/karpenter-core/pkg/events"
)

// SingleMachineConsolidation is the consolidation controller that performs single machine consolidation.
type SingleMachineConsolidation struct {
	consolidation
}

func NewSingleMachineConsolidation(clk clock.Clock, cluster *state.Cluster, kubeClient client.Client, provisioner *provisioning.Provisioner,
	cp cloudprovider.CloudProvider, recorder events.Recorder) *SingleMachineConsolidation {
	return &SingleMachineConsolidation{consolidation: makeConsolidation(clk, cluster, kubeClient, provisioner, cp, recorder)}
}

// ComputeCommand generates a deprovisioning command given deprovisionable machines
// nolint:gocyclo
func (c *SingleMachineConsolidation) ComputeCommand(ctx context.Context, candidates ...*Candidate) (Command, error) {
	if c.cluster.Consolidated() {
		return Command{}, nil
	}
	candidates, err := c.sortAndFilterCandidates(ctx, candidates)
	if err != nil {
		return Command{}, fmt.Errorf("sorting candidates, %w", err)
	}
	deprovisioningEligibleMachinesGauge.WithLabelValues(c.String()).Set(float64(len(candidates)))

	v := NewValidation(consolidationTTL, c.clock, c.cluster, c.kubeClient, c.provisioner, c.cloudProvider, c.recorder)
	for _, candidate := range candidates {
		// compute a possible consolidation option
		cmd, err := c.computeConsolidation(ctx, candidate)
		if err != nil {
			logging.FromContext(ctx).Errorf("computing consolidation %s", err)
			continue
		}
		if cmd.Action() == NoOpAction {
			continue
		}

		isValid, err := v.IsValid(ctx, cmd)
		if err != nil {
			logging.FromContext(ctx).Errorf("validating consolidation %s", err)
			continue
		}
		if !isValid {
			return Command{}, fmt.Errorf("command is no longer valid, %s", cmd)
		}
		return cmd, nil
	}
	return Command{}, nil
}
