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
	"fmt"
	"testing"

	"github.com/azure/gpu-provisioner/test/e2e/pkg/environment/common"
	"github.com/imdario/mergo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karpenterv1beta1 "sigs.k8s.io/karpenter/pkg/apis/v1beta1"
	"sigs.k8s.io/karpenter/pkg/test"
)

var env *common.Environment

func TestGPUNodeClaim(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(func() {
		env = common.NewEnvironment(t)
	})
	RunSpecs(t, "GPU NodeClaim")
}

var _ = BeforeEach(func() { env.BeforeEach() })
var _ = AfterEach(func() {
	env.AfterEach()
})

var _ = Describe("GPU NodeClaim", func() {

	It("should provision one GPU node for v1.NodeClaim", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "none",
		}

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "wctestnc1",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelInstanceTypeStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"Standard_NC12s_v3"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      karpenterv1.NodePoolLabelKey,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"kaito"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelOSStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"linux"},
						},
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

		DeferCleanup(func() {
			env.ExpectDeleted(nc)
			env.EventuallyExpectCreatedNodeClaimCount("==", 0)
			env.EventuallyExpectNodeCount("==", 0)
		})

		env.ExpectCreated(nc)
		env.EventuallyExpectCreatedNodeClaimCount("==", 1)
		env.EventuallyExpectNodeClaimsReady(nc)
		env.EventuallyExpectNodeCount("==", 1)
		_ = env.EventuallyExpectInitializedNodeCount("==", 1)[0]
	})

	It("should provision one GPU node with RAGEngine label ", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/ragengine":            "none",
		}

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "ragtestnc",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelInstanceTypeStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"Standard_NC12s_v3"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      karpenterv1.NodePoolLabelKey,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"kaito"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelOSStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"linux"},
						},
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
		DeferCleanup(func() {
			env.ExpectDeleted(nc)
			env.EventuallyExpectCreatedNodeClaimCount("==", 0)
			env.EventuallyExpectNodeCount("==", 0)
		})

		env.ExpectCreated(nc)
		env.EventuallyExpectCreatedNodeClaimCount("==", 1)
		env.EventuallyExpectNodeClaimsReady(nc)
		env.EventuallyExpectNodeCount("==", 1)
		_ = env.EventuallyExpectInitializedNodeCount("==", 1)[0]
	})

	It("should provision one GPU node for v1beta1.NodeClaim", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "none",
		}

		nc := V1beta1NodeClaim(karpenterv1beta1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "wstestnc2",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1beta1.NodeClaimSpec{
				NodeClassRef: &karpenterv1beta1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Requirements: []karpenterv1beta1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelInstanceTypeStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"Standard_NC12s_v3"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      karpenterv1.NodePoolLabelKey,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"kaito"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelOSStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"linux"},
						},
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

		DeferCleanup(func() {
			env.ExpectDeleted(nc)
			env.EventuallyExpectCreatedNodeClaimCount("==", 0)
			env.EventuallyExpectNodeCount("==", 0)
		})

		env.ExpectCreated(nc)
		env.EventuallyExpectCreatedNodeClaimCount("==", 1)
		env.EventuallyExpectNodeClaimsReady(nc)
		env.EventuallyExpectNodeCount("==", 1)
		_ = env.EventuallyExpectInitializedNodeCount("==", 1)[0]
	})
	It("terminate all resources by deleting nodeclaim", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "none",
		}

		nc := V1beta1NodeClaim(karpenterv1beta1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "wstestnc3",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1beta1.NodeClaimSpec{
				NodeClassRef: &karpenterv1beta1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Requirements: []karpenterv1beta1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelInstanceTypeStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"Standard_NC12s_v3"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      karpenterv1.NodePoolLabelKey,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"kaito"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelOSStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"linux"},
						},
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

		DeferCleanup(func() {
			env.EventuallyExpectCreatedNodeClaimCount("==", 0)
			env.EventuallyExpectNodeCount("==", 0)
		})

		env.ExpectCreated(nc)
		env.EventuallyExpectCreatedNodeClaimCount("==", 1)
		env.EventuallyExpectNodeClaimsReady(nc)
		env.EventuallyExpectNodeCount("==", 1)
		_ = env.EventuallyExpectInitializedNodeCount("==", 1)[0]

		// delete nc for triggering terminate all resrouces like node, CloudProvider Instance
		env.ExpectDeleted(nc)
	})
	It("terminate all resources by deleting node", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "none",
		}

		nc := V1beta1NodeClaim(karpenterv1beta1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "wstestnc4",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1beta1.NodeClaimSpec{
				NodeClassRef: &karpenterv1beta1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Requirements: []karpenterv1beta1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelInstanceTypeStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"Standard_NC12s_v3"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      karpenterv1.NodePoolLabelKey,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"kaito"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelOSStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"linux"},
						},
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

		DeferCleanup(func() {
			env.EventuallyExpectCreatedNodeClaimCount("==", 0)
			env.EventuallyExpectNodeCount("==", 0)
		})

		env.ExpectCreated(nc)
		env.EventuallyExpectCreatedNodeClaimCount("==", 1)
		env.EventuallyExpectNodeClaimsReady(nc)
		env.EventuallyExpectNodeCount("==", 1)
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]

		// delete node for triggering terminate all resrouces like NodeClaim, CloudProvider Instance
		env.ExpectDeleted(node)
	})

})

// V1beta1NodeClaim creates a test v1beta1.NodeClaim with defaults that can be overridden by overrides.
// Overrides are applied in order, with a last write wins semantic.
func V1beta1NodeClaim(overrides ...karpenterv1beta1.NodeClaim) *karpenterv1beta1.NodeClaim {
	override := karpenterv1beta1.NodeClaim{}
	for _, opts := range overrides {
		if err := mergo.Merge(&override, opts, mergo.WithOverride); err != nil {
			panic(fmt.Sprintf("failed to merge: %v", err))
		}
	}
	if override.Name == "" {
		override.Name = test.RandomName()
	}
	if override.Status.ProviderID == "" {
		override.Status.ProviderID = test.RandomProviderID()
	}
	if override.Spec.NodeClassRef == nil {
		override.Spec.NodeClassRef = &karpenterv1beta1.NodeClassReference{
			Name: "default",
		}
	}
	if override.Spec.Requirements == nil {
		override.Spec.Requirements = []karpenterv1beta1.NodeSelectorRequirementWithMinValues{}
	}
	return &karpenterv1beta1.NodeClaim{
		ObjectMeta: test.ObjectMeta(override.ObjectMeta),
		Spec:       override.Spec,
		Status:     override.Status,
	}
}
