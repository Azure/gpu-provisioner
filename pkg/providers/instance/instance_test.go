package instance

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v2"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/gpu-vmprovisioner/pkg/apis/v1alpha1"
	"github.com/gpu-vmprovisioner/pkg/tests"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestNewAgentPoolObject(t *testing.T) {
	testCases := []struct {
		name         string
		vmSize       string
		capacityType string
		machine      *v1alpha5.Machine
		expected     armcontainerservice.AgentPool
	}{
		{
			name:         "Machine with Storage requirement",
			vmSize:       "Standard_NC6s_v3",
			capacityType: v1alpha1.PriorityRegular,
			machine: tests.GetMachineObj(map[string]string{"test": "test"}, []v1.Taint{}, v1alpha5.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(30, resource.DecimalSI)),
				},
			}),
			expected: tests.GetAgentPoolObj(armcontainerservice.AgentPoolTypeVirtualMachineScaleSets,
				armcontainerservice.ScaleSetPriorityRegular, map[string]*string{"test": to.Ptr("test")},
				[]*string{}, 30, "Standard_NC6s_v3"),
		},
		{
			name:         "Machine with no Storage requirement",
			vmSize:       "Standard_NC6s_v3",
			capacityType: v1alpha1.PriorityRegular,
			machine: tests.GetMachineObj(map[string]string{"test": "test"}, []v1.Taint{}, v1alpha5.ResourceRequirements{
				Requests: v1.ResourceList{},
			}),
			expected: tests.GetAgentPoolObj(armcontainerservice.AgentPoolTypeVirtualMachineScaleSets,
				armcontainerservice.ScaleSetPriorityRegular, map[string]*string{"test": to.Ptr("test")},
				[]*string{}, 0, "Standard_NC6s_v3"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := newAgentPoolObject(tc.vmSize, tc.capacityType, tc.machine)
			assert.Equal(t, tc.expected.Properties.Type, result.Properties.Type)
			assert.Equal(t, tc.expected.Properties.OSDiskSizeGB, result.Properties.OSDiskSizeGB)
		})
	}
}
