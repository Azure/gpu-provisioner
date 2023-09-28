package staticprovisioner

import (
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	v1 "k8s.io/api/core/v1"
)

var (
	Sp = &v1alpha5.Provisioner{
		Spec: v1alpha5.ProvisionerSpec{
			Consolidation: &v1alpha5.Consolidation{
				Enabled: to.BoolPtr(false),
			},
			Taints: []v1.Taint{
				{
					Key:    "sku",
					Value:  "gpu",
					Effect: v1.TaintEffectNoSchedule,
				},
			},
			ProviderRef: &v1alpha5.MachineTemplateRef{
				Name: "default",
			},
		},
	}
)
