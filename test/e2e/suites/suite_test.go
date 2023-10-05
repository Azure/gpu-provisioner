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
package suites

import (
	"testing"
	"time"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/test"
	"github.com/azure/gpu-provisioner/test/e2e/pkg/environment/common"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var env *common.Environment

func TestGPU(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(func() {
		env = common.NewEnvironment(t)
	})
	RunSpecs(t, "GPU")
}

var _ = BeforeEach(func() { env.BeforeEach() })
var _ = AfterEach(func() { env.Cleanup() })
var _ = AfterEach(func() { env.AfterEach() })

var _ = Describe("GPU", func() {
	It("should provision one GPU node and one GPU Pod", func() {
		minstPodOptions := test.PodOptions{
			ObjectMeta: metav1.ObjectMeta{
				Name: "samples-fake-minst",
				Labels: map[string]string{
					"app": "samples-tf-mnist-demo",
				},
			},
			Image: "mcr.microsoft.com/oss/kubernetes/pause:3.6",
			ResourceRequirements: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					"nvidia.com/gpu": resource.MustParse("1"),
				},
			},
			Tolerations: []v1.Toleration{
				{
					Effect:   v1.TaintEffectNoSchedule,
					Operator: v1.TolerationOpEqual,
					Key:      "gpu",
				},
				{
					Effect: v1.TaintEffectNoSchedule,
					Value:  "gpu",
					Key:    "sku",
				},
			},
		}
		deployment := test.Deployment(test.DeploymentOptions{
			Replicas:   1,
			PodOptions: minstPodOptions,
		})

		var machineLabels = minstPodOptions.Labels
		machineLabels["karpenter.sh/provisioner-name"] = "default"

		machine := test.Machine(v1alpha5.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-machine",
				Labels: machineLabels,
			},
			Spec: v1alpha5.MachineSpec{
				MachineTemplateRef: &v1alpha5.MachineTemplateRef{
					Name: "test-machine",
				},
				Requirements: []v1.NodeSelectorRequirement{
					{
						Key:      v1.LabelInstanceTypeStable,
						Operator: v1.NodeSelectorOpIn,
						Values:   []string{"Standard_NC12s_v3"},
					},
					{
						Key:      "karpenter.sh/provisioner-name",
						Operator: v1.NodeSelectorOpIn,
						Values:   []string{"default"},
					},
				},
				Taints: []v1.Taint{
					{
						Key:    "sku",
						Value:  "gpu",
						Effect: v1.TaintEffectNoSchedule,
					},
				},
				Resources: v1alpha5.ResourceRequirements{
					Requests: minstPodOptions.ResourceRequirements.Limits,
				},
			},
		})
		env.ExpectCreated(machine, deployment)
		env.EventuallyExpectHealthyPodCountWithTimeout(time.Minute*15, labels.SelectorFromSet(deployment.Spec.Selector.MatchLabels), int(*deployment.Spec.Replicas))
		env.ExpectCreatedNodeCount("==", int(*deployment.Spec.Replicas))
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]
		Expect(node.Labels).To(HaveKeyWithValue("kubernetes.azure.com/accelerator", "nvidia"))
	})
})
