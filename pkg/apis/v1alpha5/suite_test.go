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

package v1alpha5_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"go.uber.org/multierr"
	v1 "k8s.io/api/core/v1"
	. "knative.dev/pkg/logging/testing"

	"github.com/Azure/karpenter/pkg/apis/v1alpha1"
	apisv1alpha5 "github.com/Azure/karpenter/pkg/apis/v1alpha5"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/scheduling"
	"github.com/aws/karpenter-core/pkg/test"
)

var ctx context.Context

func TestV1Alpha5(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "v1alpha5")
	ctx = TestContextWithLogger(t)
}

var _ = Describe("Provisioner", func() {
	var provisioner *v1alpha5.Provisioner

	BeforeEach(func() {
		provisioner = test.Provisioner()
	})

	Context("SetDefaults", func() {
		It("should default OS to linux", func() {
			SetDefaults(ctx, provisioner)
			Expect(scheduling.NewNodeSelectorRequirements(provisioner.Spec.Requirements...).Get(v1.LabelOSStable)).
				To(Equal(scheduling.NewRequirement(v1.LabelOSStable, v1.NodeSelectorOpIn, string(v1.Linux))))
		})
		It("should not default OS if set", func() {
			provisioner.Spec.Requirements = append(provisioner.Spec.Requirements,
				v1.NodeSelectorRequirement{Key: v1.LabelOSStable, Operator: v1.NodeSelectorOpDoesNotExist})
			SetDefaults(ctx, provisioner)
			Expect(scheduling.NewNodeSelectorRequirements(provisioner.Spec.Requirements...).Get(v1.LabelOSStable)).
				To(Equal(scheduling.NewRequirement(v1.LabelOSStable, v1.NodeSelectorOpDoesNotExist)))
		})
		It("should default architecture to amd64", func() {
			SetDefaults(ctx, provisioner)
			Expect(scheduling.NewNodeSelectorRequirements(provisioner.Spec.Requirements...).Get(v1.LabelArchStable)).
				To(Equal(scheduling.NewRequirement(v1.LabelArchStable, v1.NodeSelectorOpIn, v1alpha5.ArchitectureAmd64)))
		})
		It("should not default architecture if set", func() {
			provisioner.Spec.Requirements = append(provisioner.Spec.Requirements,
				v1.NodeSelectorRequirement{Key: v1.LabelArchStable, Operator: v1.NodeSelectorOpDoesNotExist})
			SetDefaults(ctx, provisioner)
			Expect(scheduling.NewNodeSelectorRequirements(provisioner.Spec.Requirements...).Get(v1.LabelArchStable)).
				To(Equal(scheduling.NewRequirement(v1.LabelArchStable, v1.NodeSelectorOpDoesNotExist)))
		})
		It("should default capacity-type to regular", func() {
			SetDefaults(ctx, provisioner)
			Expect(scheduling.NewNodeSelectorRequirements(provisioner.Spec.Requirements...).Get(v1alpha5.LabelCapacityType)).
				To(Equal(scheduling.NewRequirement(v1alpha5.LabelCapacityType, v1.NodeSelectorOpIn, v1alpha1.PriorityRegular)))
		})
		It("should not default capacity-type if set", func() {
			provisioner.Spec.Requirements = append(provisioner.Spec.Requirements,
				v1.NodeSelectorRequirement{Key: v1alpha5.LabelCapacityType, Operator: v1.NodeSelectorOpDoesNotExist})
			SetDefaults(ctx, provisioner)
			Expect(scheduling.NewNodeSelectorRequirements(provisioner.Spec.Requirements...).Get(v1alpha5.LabelCapacityType)).
				To(Equal(scheduling.NewRequirement(v1alpha5.LabelCapacityType, v1.NodeSelectorOpDoesNotExist)))
		})

		// TODO: add tests for default SKU family, etc.
	})

	Context("Validate", func() {

		It("should validate", func() {
			Expect(provisioner.Validate(ctx)).To(Succeed())
		})
		It("should succeed if provider undefined", func() {
			provisioner.Spec.Provider = nil
			provisioner.Spec.ProviderRef = &v1alpha5.MachineTemplateRef{
				Kind: "NodeTemplate",
				Name: "default",
			}
			Expect(provisioner.Validate(ctx)).To(Succeed())
		})

		// TODO: add tests
	})
})

func SetDefaults(ctx context.Context, provisioner *v1alpha5.Provisioner) {
	prov := apisv1alpha5.Provisioner(*provisioner)
	prov.SetDefaults(ctx)
	*provisioner = v1alpha5.Provisioner(prov)
}

func Validate(ctx context.Context, provisioner *v1alpha5.Provisioner) error {
	return multierr.Combine(
		lo.ToPtr(apisv1alpha5.Provisioner(*provisioner)).Validate(ctx),
		provisioner.Validate(ctx),
	)
}
