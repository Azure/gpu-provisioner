package staticprovisioner

import (
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
)

var (
	Sp = &v1alpha5.Provisioner{
		Spec: v1alpha5.ProvisionerSpec{
			//Requirements: []v1.NodeSelectorRequirement{
			//	v1.NodeSelectorRequirement{
			//		Key:      "karpenter.k8s.azure/sku-family",
			//		Operator: "In",
			//		Values:   []string{"N"},
			//	},
			//},

			ProviderRef: &v1alpha5.MachineTemplateRef{
				Name: "default",
			},
		},
	}
)
