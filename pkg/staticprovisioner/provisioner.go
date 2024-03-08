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
