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

package tests

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetMachineObj(name string, labels map[string]string, taints []v1.Taint, resource v1alpha5.ResourceRequirements, req []v1.NodeSelectorRequirement) *v1alpha5.Machine {
	return &v1alpha5.Machine{
		ObjectMeta: v12.ObjectMeta{
			Name:      name,
			Namespace: "machine-ns",
			Labels:    labels,
		},
		Spec: v1alpha5.MachineSpec{
			Resources:          resource,
			Requirements:       req,
			MachineTemplateRef: &v1alpha5.MachineTemplateRef{},
			Taints:             taints,
		},
	}
}

func GetAgentPoolObj(apType armcontainerservice.AgentPoolType, capacityType armcontainerservice.ScaleSetPriority,
	labels map[string]*string, taints []*string, storage int32, vmSize string) armcontainerservice.AgentPool {
	return armcontainerservice.AgentPool{
		Properties: &armcontainerservice.ManagedClusterAgentPoolProfileProperties{
			NodeLabels:       labels,
			NodeTaints:       taints,
			Type:             to.Ptr(apType),
			VMSize:           to.Ptr(vmSize),
			OSType:           to.Ptr(armcontainerservice.OSTypeLinux),
			Count:            to.Ptr(int32(1)),
			ScaleSetPriority: to.Ptr(capacityType),
			OSDiskSizeGB:     to.Ptr(storage),
		},
	}
}

func GetAgentPoolObjWithName(apName string, apId string, vmSize string) armcontainerservice.AgentPool {
	return armcontainerservice.AgentPool{
		Name: &apName,
		ID:   &apId,
		Properties: &armcontainerservice.ManagedClusterAgentPoolProfileProperties{
			VMSize: &vmSize,
			NodeLabels: map[string]*string{
				"test": to.Ptr("test"),
			},
		},
	}
}
func GetNodeList(nodes []v1.Node) *v1.NodeList {
	return &v1.NodeList{
		Items: nodes,
	}
}

var (
	ReadyNode = v1.Node{
		ObjectMeta: v12.ObjectMeta{
			Name: "aks-agentpool0-20562481-vmss_0",
			Labels: map[string]string{
				"agentpool":                      "agentpool0",
				"kubernetes.azure.com/agentpool": "agentpool0",
			},
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}
)

func NotFoundAzError() *azcore.ResponseError {
	return &azcore.ResponseError{ErrorCode: "NotFound"}
}
