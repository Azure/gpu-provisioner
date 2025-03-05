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

package fake

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func GetNodeClaimObj(name string, labels map[string]string, taints []v1.Taint, resource karpenterv1.ResourceRequirements, req []v1.NodeSelectorRequirement) *karpenterv1.NodeClaim {
	requirements := lo.Map(req, func(v1Requirements v1.NodeSelectorRequirement, _ int) karpenterv1.NodeSelectorRequirementWithMinValues {
		return karpenterv1.NodeSelectorRequirementWithMinValues{
			NodeSelectorRequirement: v1.NodeSelectorRequirement{
				Key:      v1Requirements.Key,
				Operator: v1Requirements.Operator,
				Values:   v1Requirements.Values,
			},
			MinValues: to.Ptr(int(1)),
		}
	})

	labels["kaito.sh/workspace"] = "none"
	labels[karpenterv1.NodePoolLabelKey] = "kaito"
	return &karpenterv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "nodeclaim-ns",
			Labels:    labels,
		},
		Spec: karpenterv1.NodeClaimSpec{
			Resources:    resource,
			Requirements: requirements,
			NodeClassRef: &karpenterv1.NodeClassReference{},
			Taints:       taints,
		},
		Status: karpenterv1.NodeClaimStatus{
			ProviderID: fmt.Sprintf("azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-%s-20562481-vmss/virtualMachines/0", name),
		},
	}
}

func GetNodeClaimObjWithoutProviderID(name string, labels map[string]string, taints []v1.Taint, resource karpenterv1.ResourceRequirements, req []v1.NodeSelectorRequirement) *karpenterv1.NodeClaim {
	requirements := lo.Map(req, func(v1Requirements v1.NodeSelectorRequirement, _ int) karpenterv1.NodeSelectorRequirementWithMinValues {
		return karpenterv1.NodeSelectorRequirementWithMinValues{
			NodeSelectorRequirement: v1.NodeSelectorRequirement{
				Key:      v1Requirements.Key,
				Operator: v1Requirements.Operator,
				Values:   v1Requirements.Values,
			},
			MinValues: to.Ptr(int(1)),
		}
	})
	return &karpenterv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "nodeclaim-ns",
			Labels:    labels,
		},
		Spec: karpenterv1.NodeClaimSpec{
			Resources:    resource,
			Requirements: requirements,
			NodeClassRef: &karpenterv1.NodeClassReference{},
			Taints:       taints,
		},
	}
}
