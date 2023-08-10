package staticprovisioner

import (
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	v1 "k8s.io/api/core/v1"
)

var (
	Sp = &v1alpha5.Provisioner{
		Spec: v1alpha5.ProvisionerSpec{
			Requirements: []v1.NodeSelectorRequirement{
				v1.NodeSelectorRequirement{
					Key:      "karpenter.k8s.azure/sku-family",
					Operator: "In",
					Values:   []string{"D"},
				},

				v1.NodeSelectorRequirement{
					Key:      "karpenter.k8s.azure/sku-storage-premium-capable",
					Operator: "In",
					Values:   []string{"true"},
				},

				v1.NodeSelectorRequirement{
					Key:      "karpenter.k8s.azure/sku-cpu",
					Operator: "Lt",
					Values:   []string{"33"},
				},
			},

			ProviderRef: &v1alpha5.MachineTemplateRef{
				Name: "default",
			},
		},
	}
)
