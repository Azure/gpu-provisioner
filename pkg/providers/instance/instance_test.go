package instance

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/azure/gpu-provisioner/pkg/fake"
	"github.com/azure/gpu-provisioner/pkg/tests"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNewAgentPoolObject(t *testing.T) {
	testCases := []struct {
		name     string
		vmSize   string
		machine  *v1alpha5.Machine
		expected armcontainerservice.AgentPool
	}{
		{
			name:   "Machine with Storage requirement",
			vmSize: "Standard_NC6s_v3",
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
			name:   "Machine with no Storage requirement",
			vmSize: "Standard_NC6s_v3",
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
			result := newAgentPoolObject(tc.vmSize, tc.machine)
			assert.Equal(t, tc.expected.Properties.Type, result.Properties.Type)
			assert.Equal(t, tc.expected.Properties.OSDiskSizeGB, result.Properties.OSDiskSizeGB)
		})
	}
}

func TestGet(t *testing.T) {
	testCases := []struct {
		name              string
		id                string
		mockAgentPool     armcontainerservice.AgentPool
		mockAgentPoolResp func(ap armcontainerservice.AgentPool) armcontainerservice.AgentPoolsClientGetResponse
		callK8sMocks      func(c *fake.MockClient)
		expectedError     error
	}{
		{
			name:          "Successfully Get instance from agent pool",
			id:            "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss/virtualMachines/0",
			mockAgentPool: tests.GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3"),
			mockAgentPoolResp: func(ap armcontainerservice.AgentPool) armcontainerservice.AgentPoolsClientGetResponse {
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: ap}
			},
			callK8sMocks: func(c *fake.MockClient) {
				nodeList := tests.GetNodeList([]v1.Node{tests.ReadyNode})
				relevantMap := c.CreateMapWithType(nodeList)
				//insert node objects into the map
				for _, obj := range nodeList.Items {
					n := obj
					objKey := client.ObjectKeyFromObject(&n)

					relevantMap[objKey] = &n
				}

				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(nil)
			},
		},
		{
			name: "Fail to get instance because agentPool.Get returns a failure",
			id:   "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss/virtualMachines/0",
			mockAgentPoolResp: func(ap armcontainerservice.AgentPool) armcontainerservice.AgentPoolsClientGetResponse {
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: ap}
			},
			expectedError: errors.New("Failed to get agent pool"),
		},
		{
			name:          "Fail to get instance because agent pool ID cannot be parsed properly",
			id:            "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/virtualMachines/0",
			expectedError: errors.New("getting agentpool name, id does not match the regxp for ParseAgentPoolNameFromID"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)
			if tc.mockAgentPoolResp != nil {
				agentPoolMocks.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), "agentpool0", gomock.Any()).Return(tc.mockAgentPoolResp(tc.mockAgentPool), tc.expectedError)
			}

			mockK8sClient := fake.NewClient()
			if tc.callK8sMocks != nil {
				tc.callK8sMocks(mockK8sClient)
			}

			p := createTestProvider(agentPoolMocks, mockK8sClient)

			instance, err := p.Get(context.Background(), tc.id)

			if tc.expectedError == nil {
				assert.NoError(t, err, "Not expected to return error")
				assert.NotNil(t, instance, "Response instance should not be nil")
				assert.Equal(t, tc.mockAgentPool.Name, instance.Name, "Instance name should be same as the agent pool")
				assert.Equal(t, tc.mockAgentPool.Properties.VMSize, instance.Type, "Instance type should be same as agent pool's vm size")
			} else {
				assert.Contains(t, err.Error(), tc.expectedError.Error())
			}
		})
	}
}

func TestFromAgentPoolToInstance(t *testing.T) {
	testCases := []struct {
		name          string
		callK8sMocks  func(c *fake.MockClient)
		mockAgentPool armcontainerservice.AgentPool
		isInstanceNil bool
		expectedError error
	}{
		{
			name:          "Successfully Get instance from agent pool",
			mockAgentPool: tests.GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3"),
			callK8sMocks: func(c *fake.MockClient) {
				nodeList := tests.GetNodeList([]v1.Node{tests.ReadyNode})
				relevantMap := c.CreateMapWithType(nodeList)
				//insert node objects into the map
				for _, obj := range nodeList.Items {
					n := obj
					objKey := client.ObjectKeyFromObject(&n)

					relevantMap[objKey] = &n
				}

				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(nil)
			},
		},
		{
			name:          "Fail to get instance from agent pool because node is nil",
			mockAgentPool: tests.GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3"),
			callK8sMocks: func(c *fake.MockClient) {

				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(nil)
			},
			isInstanceNil: true,
		},
		{
			name:          "Fail to get instance from agent pool due to error in retrieving node list",
			mockAgentPool: tests.GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3"),
			callK8sMocks: func(c *fake.MockClient) {
				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(errors.New("Fail to get node list"))
			},
			expectedError: errors.New("Fail to get node list"),
		},
		{
			name:          "Fail to get instance from agent pool due to malformed id",
			mockAgentPool: tests.GetAgentPoolObjWithName("agentpool0", "/subscriptions/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3"),
			expectedError: errors.New("id does not match the regxp for ParseSubIDFromID"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)

			mockK8sClient := fake.NewClient()
			if tc.callK8sMocks != nil {
				tc.callK8sMocks(mockK8sClient)
			}

			p := createTestProvider(agentPoolMocks, mockK8sClient)

			instance, err := p.fromAgentPoolToInstance(context.Background(), &tc.mockAgentPool)

			if tc.expectedError == nil {
				assert.NoError(t, err, "Not expected to return error")
				if !tc.isInstanceNil {
					assert.NotNil(t, instance, "Response instance should not be nil")
					assert.Equal(t, tc.mockAgentPool.Name, instance.Name, "Instance name should be same as the agent pool")
					assert.Equal(t, tc.mockAgentPool.Properties.VMSize, instance.Type, "Instance type should be same as agent pool's vm size")
				} else {
					assert.Nil(t, instance, "Response instance should be nil")
				}
			} else {
				assert.Contains(t, err.Error(), tc.expectedError.Error())
			}

		})
	}
}

func TestDelete(t *testing.T) {
	testCases := []struct {
		name              string
		id                string
		mockAgentPoolResp func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error)
		expectedError     error
	}{
		{
			name: "Successfully delete instance",
			id:   "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss/virtualMachines/0",
			mockAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
				delResp := armcontainerservice.AgentPoolsClientDeleteResponse{}
				resp := http.Response{Status: "200 OK", StatusCode: http.StatusOK, Body: http.NoBody}

				mockHandler.EXPECT().Done().Return(true).Times(3)
				mockHandler.EXPECT().Result(gomock.Any(), gomock.Any()).Return(nil)

				pollingOptions := &runtime.NewPollerOptions[armcontainerservice.AgentPoolsClientDeleteResponse]{
					Handler:  mockHandler,
					Response: &delResp,
				}

				p, err := runtime.NewPoller(&resp, runtime.NewPipeline("", "", runtime.PipelineOptions{}, nil), pollingOptions)
				return p, err
			},
		},
		{
			name: "Successfully deletes instance because poller returns a 404 not found error",
			id:   "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss/virtualMachines/0",
			mockAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
				delResp := armcontainerservice.AgentPoolsClientDeleteResponse{}
				resp := http.Response{StatusCode: http.StatusBadRequest, Body: http.NoBody}

				mockHandler.EXPECT().Done().Return(false)
				mockHandler.EXPECT().Poll(gomock.Any()).Return(&resp, tests.NotFoundAzError())

				pollingOptions := &runtime.NewPollerOptions[armcontainerservice.AgentPoolsClientDeleteResponse]{
					Handler:  mockHandler,
					Response: &delResp,
				}

				p, err := runtime.NewPoller(&resp, runtime.NewPipeline("", "", runtime.PipelineOptions{}, nil), pollingOptions)
				return p, err
			},
		},
		{
			name: "Fail to delete instance because poller returns error",
			id:   "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss/virtualMachines/0",
			mockAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
				delResp := armcontainerservice.AgentPoolsClientDeleteResponse{}
				resp := http.Response{StatusCode: http.StatusBadRequest, Body: http.NoBody}

				mockHandler.EXPECT().Done().Return(false)
				mockHandler.EXPECT().Poll(gomock.Any()).Return(&resp, errors.New("Failed to fetch latest status of operation"))

				pollingOptions := &runtime.NewPollerOptions[armcontainerservice.AgentPoolsClientDeleteResponse]{
					Handler:  mockHandler,
					Response: &delResp,
				}

				p, err := runtime.NewPoller(&resp, runtime.NewPipeline("", "", runtime.PipelineOptions{}, nil), pollingOptions)
				return p, err
			},
			expectedError: errors.New("Failed to fetch latest status of operation"),
		},
		{
			name: "Successfully delete instance because agentPool.Delete returns a NotFound error",
			id:   "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss/virtualMachines/0",
			mockAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
				return nil, tests.NotFoundAzError()
			},
		},
		{
			name: "Fail to delete instance because agentPool.Delete returns a failure",
			id:   "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss/virtualMachines/0",
			mockAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
				return nil, errors.New("Failed to delete agent pool")
			},
			expectedError: errors.New("Failed to delete agent pool"),
		},
		{
			name:          "Fail to delete instance because agent pool ID cannot be parsed properly",
			id:            "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/virtualMachines/0",
			expectedError: errors.New("getting agentpool name, id does not match the regxp for ParseAgentPoolNameFromID"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)
			if tc.mockAgentPoolResp != nil {
				mockHandler := fake.NewMockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse](mockCtrl)

				p, err := tc.mockAgentPoolResp(mockHandler)
				agentPoolMocks.EXPECT().BeginDelete(gomock.Any(), gomock.Any(), gomock.Any(), "agentpool0", gomock.Any()).Return(p, err)
			}

			mockK8sClient := fake.NewClient()
			p := createTestProvider(agentPoolMocks, mockK8sClient)

			err := p.Delete(context.Background(), tc.id)

			if tc.expectedError == nil {
				assert.NoError(t, err, "Not expected to return error")
			} else {
				assert.Contains(t, err.Error(), tc.expectedError.Error())
			}
		})
	}
}

func createTestProvider(agentPoolsAPIMocks *fake.MockAgentPoolsAPI, mockK8sClient *fake.MockClient) *Provider {
	mockAzClient := NewAZClientFromAPI(agentPoolsAPIMocks, nil)
	return NewProvider(mockAzClient, mockK8sClient, nil, nil, "testRG", "nodeRG", "testCluster")
}
