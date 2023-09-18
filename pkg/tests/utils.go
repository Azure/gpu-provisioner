package tests

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetMachineObj(labels map[string]string, taints []v1.Taint, resource v1alpha5.ResourceRequirements) *v1alpha5.Machine {
	return &v1alpha5.Machine{
		ObjectMeta: v12.ObjectMeta{
			Name:      "machine-test",
			Namespace: "machine-ns",
			Labels:    labels,
		},
		Spec: v1alpha5.MachineSpec{
			Resources:          resource,
			Requirements:       []v1.NodeSelectorRequirement{},
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
