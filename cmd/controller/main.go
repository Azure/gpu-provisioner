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

package main

import (
	"github.com/samber/lo"

	"github.com/gpu-vmprovisioner/pkg/cloudprovider"
	"github.com/gpu-vmprovisioner/pkg/operator"

	"github.com/aws/karpenter-core/pkg/cloudprovider/metrics"
	corecontrollers "github.com/aws/karpenter-core/pkg/controllers"
	"github.com/aws/karpenter-core/pkg/controllers/state"
	coreoperator "github.com/aws/karpenter-core/pkg/operator"
)

func main() {
	ctx, op := operator.NewOperator(coreoperator.NewOperator())
	azureCloudProvider := cloudprovider.New(
		op.InstanceTypesProvider,
		op.InstanceProvider,
		op.GetClient(),
	)

	lo.Must0(op.AddHealthzCheck("cloud-provider", azureCloudProvider.LivenessProbe))
	cloudProvider := metrics.Decorate(azureCloudProvider)

	op.
		WithControllers(ctx, corecontrollers.NewControllers(
			ctx,
			op.Clock,
			op.GetClient(),
			op.KubernetesInterface,
			state.NewCluster(op.Clock, op.GetClient(), cloudProvider),
			op.EventRecorder,
			cloudProvider,
		)...).
		Start(ctx)
}
