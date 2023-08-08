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

package integration_test

import (
	"k8s.io/apimachinery/pkg/labels"
	"knative.dev/pkg/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/test"
	"github.com/aws/karpenter/pkg/apis/settings"
	"github.com/aws/karpenter/pkg/apis/v1alpha1"
	awstest "github.com/aws/karpenter/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Emptiness", func() {
	It("should terminate an empty node", func() {
		provider := awstest.AWSNodeTemplate(v1alpha1.AWSNodeTemplateSpec{AWS: v1alpha1.AWS{
			SecurityGroupSelector: map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
			SubnetSelector:        map[string]string{"karpenter.sh/discovery": settings.FromContext(env.Context).ClusterName},
		}})
		provisioner := test.Provisioner(test.ProvisionerOptions{
			ProviderRef:          &v1alpha5.MachineTemplateRef{Name: provider.Name},
			TTLSecondsAfterEmpty: ptr.Int64(1e6), // A really long timeframe so that we set the Empty Status Condition
		})

		const numPods = 1
		deployment := test.Deployment(test.DeploymentOptions{Replicas: numPods})

		By("kicking off provisioning for a deployment")
		env.ExpectCreated(provider, provisioner, deployment)
		machine := env.EventuallyExpectCreatedMachineCount("==", 1)[0]
		node := env.EventuallyExpectCreatedNodeCount("==", 1)[0]
		env.EventuallyExpectHealthyPodCount(labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels), numPods)

		By("making the machine empty")
		persisted := deployment.DeepCopy()
		deployment.Spec.Replicas = ptr.Int32(0)
		Expect(env.Client.Patch(env, deployment, client.MergeFrom(persisted))).To(Succeed())

		By("waiting for the machine emptiness status condition to propagate")
		EventuallyWithOffset(1, func(g Gomega) {
			g.Expect(env.Client.Get(env, client.ObjectKeyFromObject(machine), machine)).To(Succeed())
			g.Expect(machine.StatusConditions().GetCondition(v1alpha5.MachineEmpty)).ToNot(BeNil())
			g.Expect(machine.StatusConditions().GetCondition(v1alpha5.MachineEmpty).IsTrue()).To(BeTrue())
		}).Should(Succeed())

		By("waiting for the machine to deprovision when past its TTLSecondsAfterEmpty of 0")
		provisioner.Spec.TTLSecondsAfterEmpty = ptr.Int64(0)
		env.ExpectUpdated(provisioner)

		env.EventuallyExpectNotFound(machine, node)
	})
})
