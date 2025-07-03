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

	"github.com/azure/gpu-provisioner/test/e2e/pkg/environment/common"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
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
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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
	It("terminate all resources by deleting nodeclaim", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "none",
		}

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "wctestnc3",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "wctestnc4",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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

	It("should provision one GPU node with Azure Linux via label", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "azure-linux-test",
			"kaito.sh/node-image-family":    "AzureLinux",
		}

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "azlinuxnc",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]

		// Verify the node is running Azure Linux
		Expect(node.Status.NodeInfo.OSImage).To(Or(ContainSubstring("Azure"), ContainSubstring("Mariner")),
			"Node should be running Azure Linux, got OS: %s", node.Status.NodeInfo.OSImage)
	})

	It("should provision one GPU node with Azure Linux via annotation", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "azure-linux-annotation-test",
		}

		nodeClaimAnnotations := map[string]string{
			"kaito.sh/node-image-family": "AzureLinux",
		}

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "azlinuxannot",
				Labels:      nodeClaimLabels,
				Annotations: nodeClaimAnnotations,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]

		// Verify the node is running Azure Linux
		Expect(node.Status.NodeInfo.OSImage).To(Or(ContainSubstring("Azure"), ContainSubstring("Mariner")),
			"Node should be running Azure Linux, got OS: %s", node.Status.NodeInfo.OSImage)
	})

	It("should handle case-insensitive Azure Linux image family values", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "azure-linux-case-test",
			"kaito.sh/node-image-family":    "azurelinux", // lowercase
		}

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "azlinuxcase",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]

		// Verify the node is running Azure Linux (case-insensitive support)
		Expect(node.Status.NodeInfo.OSImage).To(Or(ContainSubstring("Azure"), ContainSubstring("Mariner")),
			"Node should be running Azure Linux with case-insensitive support, got OS: %s", node.Status.NodeInfo.OSImage)
	})

	It("should fallback to Ubuntu when invalid image family is specified", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "fallback-test",
			"kaito.sh/node-image-family":    "InvalidImageFamily",
		}

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "fallbacknc",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]

		// Verify the node falls back to Ubuntu when invalid image family is specified
		Expect(node.Status.NodeInfo.OSImage).To(ContainSubstring("Ubuntu"),
			"Node should fallback to Ubuntu when invalid image family is specified, got OS: %s", node.Status.NodeInfo.OSImage)
	})

	It("should terminate node when delete triggered", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "none",
		}

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "wctestnc5",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]

		// delete node for triggering terminate all resrouces like NodeClaim, CloudProvider Instance
		env.ExpectDeleted(node)
	})

	It("should terminate node when delete triggered (Azure Linux)", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "azure-linux-test",
			"kaito.sh/node-image-family":    "AzureLinux",
		}

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "azlinuxnc2",
				Labels: nodeClaimLabels,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]

		// delete node for triggering terminate all resrouces like NodeClaim, CloudProvider Instance
		env.ExpectDeleted(node)
	})

	It("should terminate node when delete triggered (Azure Linux - annotation)", func() {
		nodeClaimLabels := map[string]string{
			"karpenter.sh/provisioner-name": "default",
			"kaito.sh/workspace":            "azure-linux-annotation-test",
		}

		nodeClaimAnnotations := map[string]string{
			"kaito.sh/node-image-family": "AzureLinux",
		}

		nc := test.NodeClaim(karpenterv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "azlinuxann2",
				Labels:      nodeClaimLabels,
				Annotations: nodeClaimAnnotations,
			},
			Spec: karpenterv1.NodeClaimSpec{
				NodeClassRef: &karpenterv1.NodeClassReference{
					Name: "default",
					Kind: "AKSNodeClass",
				},
				Resources: karpenterv1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(120*1024*1024*1024, resource.DecimalSI)),
					},
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
		node := env.EventuallyExpectInitializedNodeCount("==", 1)[0]

		// delete node for triggering terminate all resrouces like NodeClaim, CloudProvider Instance
		env.ExpectDeleted(node)
	})

})
