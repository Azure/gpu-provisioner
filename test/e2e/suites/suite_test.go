/*
       Copyright (c) Microsoft Corporation.
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

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/test"
	"github.com/azure/gpu-provisioner/test/e2e/pkg/environment/common"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var env *common.Environment

func TestGPUMachine(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(func() {
		env = common.NewEnvironment(t)
	})
	RunSpecs(t, "GPU Machine")
}

var _ = BeforeEach(func() { env.BeforeEach() })
var _ = AfterEach(func() { env.Cleanup() })
var _ = AfterEach(func() { env.AfterEach() })

var _ = Describe("GPU Machine", func() {
	It("should provision one GPU node ", func() {
		machineLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "none",
		}

		machine := test.Machine(v1alpha5.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "testmachine",
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
			},
		})
		env.ExpectCreated(machine)
		env.EventuallyExpectCreatedMachineCount("==", 1)
		env.EventuallyExpectMachinesReady(machine)
		env.EventuallyExpectNodeCount("==", 1)
		_ = env.EventuallyExpectInitializedNodeCount("==", 1)[0]
	})
})
