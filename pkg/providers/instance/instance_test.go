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

package instance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/azure/gpu-provisioner/pkg/fake"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func TestNewAgentPoolObject(t *testing.T) {
	testCases := []struct {
		name        string
		vmSize      string
		nodeClaim   *karpenterv1.NodeClaim
		expected    armcontainerservice.AgentPool
		expectedErr bool
	}{
		{
			name:   "NodeClaim with Storage requirement",
			vmSize: "Standard_NC6s_v3",
			nodeClaim: fake.GetNodeClaimObj("nodeclaim-test", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI)),
				},
			}, []v1.NodeSelectorRequirement{}),
			expected: GetAgentPoolObj(armcontainerservice.AgentPoolTypeVirtualMachineScaleSets,
				armcontainerservice.ScaleSetPriorityRegular, map[string]*string{"test": lo.ToPtr("test")},
				[]*string{}, 30, "Standard_NC6s_v3"),
			expectedErr: false,
		},
		{
			name:   "NodeClaim with no Storage requirement",
			vmSize: "Standard_NC6s_v3",
			nodeClaim: fake.GetNodeClaimObj("nodeclaim-test", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{
				Requests: v1.ResourceList{},
			}, []v1.NodeSelectorRequirement{}),
			expected:    armcontainerservice.AgentPool{},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := newAgentPoolObject(tc.vmSize, tc.nodeClaim)
			if tc.expectedErr {
				assert.EqualError(t, err, fmt.Sprintf("storage request of nodeclaim(%s) should be more than 0", tc.nodeClaim.Name))
				return
			}
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
			mockAgentPool: GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3"),
			mockAgentPoolResp: func(ap armcontainerservice.AgentPool) armcontainerservice.AgentPoolsClientGetResponse {
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: ap}
			},
			callK8sMocks: func(c *fake.MockClient) {
				nodeList := GetNodeList([]v1.Node{ReadyNode})
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
			mockAgentPool: GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3"),
			callK8sMocks: func(c *fake.MockClient) {
				nodeList := GetNodeList([]v1.Node{ReadyNode})
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
			mockAgentPool: GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3"),
			callK8sMocks: func(c *fake.MockClient) {

				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(nil)
			},
			isInstanceNil: true,
		},
		{
			name:          "Fail to get instance from agent pool due to error in retrieving node list",
			mockAgentPool: GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3"),
			callK8sMocks: func(c *fake.MockClient) {
				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(errors.New("Fail to get node list"))
			},
			expectedError: errors.New("Fail to get node list"),
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

			instance, err := p.fromRegisteredAgentPoolToInstance(context.Background(), &tc.mockAgentPool)
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
		apName            string
		mockAgentPoolGet  func() (armcontainerservice.AgentPoolsClientGetResponse, error)
		mockAgentPoolResp func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error)
		expectedError     error
	}{
		{
			name:   "Successfully delete instance",
			apName: "agentpool0",
			mockAgentPoolGet: func() (armcontainerservice.AgentPoolsClientGetResponse, error) {
				ap := GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3")
				ap.Properties.ProvisioningState = lo.ToPtr("Succeeded")
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: ap}, nil
			},
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
			name:   "Successfully deletes instance because poller returns a 404 not found error",
			apName: "agentpool0",
			mockAgentPoolGet: func() (armcontainerservice.AgentPoolsClientGetResponse, error) {
				ap := GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3")
				ap.Properties.ProvisioningState = lo.ToPtr("Succeeded")
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: ap}, nil
			},
			mockAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
				delResp := armcontainerservice.AgentPoolsClientDeleteResponse{}
				resp := http.Response{StatusCode: http.StatusBadRequest, Body: http.NoBody}

				mockHandler.EXPECT().Done().Return(false)
				mockHandler.EXPECT().Poll(gomock.Any()).Return(&resp, NotFoundAzError())

				pollingOptions := &runtime.NewPollerOptions[armcontainerservice.AgentPoolsClientDeleteResponse]{
					Handler:  mockHandler,
					Response: &delResp,
				}

				p, err := runtime.NewPoller(&resp, runtime.NewPipeline("", "", runtime.PipelineOptions{}, nil), pollingOptions)
				return p, err
			},
			expectedError: errors.New("nodeclaim not found"),
		},
		{
			name:   "Fail to delete instance because poller returns error",
			apName: "agentpool0",
			mockAgentPoolGet: func() (armcontainerservice.AgentPoolsClientGetResponse, error) {
				ap := GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3")
				ap.Properties.ProvisioningState = lo.ToPtr("Succeeded")
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: ap}, nil
			},
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
			name:   "Successfully delete instance because agentPool.Delete returns a NotFound error",
			apName: "agentpool0",
			mockAgentPoolGet: func() (armcontainerservice.AgentPoolsClientGetResponse, error) {
				ap := GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3")
				ap.Properties.ProvisioningState = lo.ToPtr("Succeeded")
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: ap}, nil
			},
			mockAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
				return nil, NotFoundAzError()
			},
			expectedError: errors.New("nodeclaim not found"),
		},
		{
			name:   "Fail to delete instance because agentPool.Delete returns a failure",
			apName: "agentpool0",
			mockAgentPoolGet: func() (armcontainerservice.AgentPoolsClientGetResponse, error) {
				ap := GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3")
				ap.Properties.ProvisioningState = lo.ToPtr("Succeeded")
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: ap}, nil
			},
			mockAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
				return nil, errors.New("Failed to delete agent pool")
			},
			expectedError: errors.New("Failed to delete agent pool"),
		},
		{
			name:   "Successfully delete instance when agent pool is already deleting",
			apName: "agentpool0",
			mockAgentPoolGet: func() (armcontainerservice.AgentPoolsClientGetResponse, error) {
				ap := GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3")
				ap.Properties.ProvisioningState = lo.ToPtr("Deleting")
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: ap}, nil
			},
		},
		{
			name:   "Successfully delete instance when agent pool get returns NotFound error",
			apName: "agentpool0",
			mockAgentPoolGet: func() (armcontainerservice.AgentPoolsClientGetResponse, error) {
				return armcontainerservice.AgentPoolsClientGetResponse{}, errors.New("Agent Pool not found")
			},
			expectedError: errors.New("nodeclaim not found"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)

			// Mock Get call if specified
			if tc.mockAgentPoolGet != nil {
				getResp, getErr := tc.mockAgentPoolGet()
				agentPoolMocks.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), "agentpool0", gomock.Any()).Return(getResp, getErr).MaxTimes(1)
			}

			// Mock Delete call if specified
			if tc.mockAgentPoolResp != nil {
				mockHandler := fake.NewMockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse](mockCtrl)

				p, err := tc.mockAgentPoolResp(mockHandler)
				agentPoolMocks.EXPECT().BeginDelete(gomock.Any(), gomock.Any(), gomock.Any(), "agentpool0", gomock.Any()).Return(p, err).MaxTimes(1)
			}

			mockK8sClient := fake.NewClient()
			p := createTestProvider(agentPoolMocks, mockK8sClient)

			err := p.Delete(context.Background(), tc.apName)

			if tc.expectedError == nil {
				assert.NoError(t, err, "Not expected to return error")
			} else {
				assert.Error(t, err, "Expected to return error")
				assert.Contains(t, err.Error(), tc.expectedError.Error(), "Error message should contain expected text")
			}
		})
	}
}

func TestList(t *testing.T) {
	testCases := []struct {
		name              string
		mockAgentPoolList func() []*armcontainerservice.AgentPool
		mockAgentPoolResp func(apList []*armcontainerservice.AgentPool) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse]
		callK8sMocks      func(c *fake.MockClient)
		expectedError     error
	}{
		{
			name: "Successfully list instances",
			mockAgentPoolList: func() []*armcontainerservice.AgentPool {
				ap := GetAgentPoolObjWithName("agentpool0", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3")
				ap1 := GetAgentPoolObjWithName("agentpool1", "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", "Standard_NC6s_v3")

				return []*armcontainerservice.AgentPool{
					&ap, &ap1,
				}
			},
			mockAgentPoolResp: func(apList []*armcontainerservice.AgentPool) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse] {
				return runtime.NewPager(runtime.PagingHandler[armcontainerservice.AgentPoolsClientListResponse]{
					More: func(page armcontainerservice.AgentPoolsClientListResponse) bool {
						return false
					},
					Fetcher: func(ctx context.Context, page *armcontainerservice.AgentPoolsClientListResponse) (armcontainerservice.AgentPoolsClientListResponse, error) {
						return armcontainerservice.AgentPoolsClientListResponse{
							AgentPoolListResult: armcontainerservice.AgentPoolListResult{
								Value: apList,
							},
						}, nil
					},
				})
			},
			callK8sMocks: func(c *fake.MockClient) {
				nodeList := GetNodeList([]v1.Node{ReadyNode})
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
			name: "Fail to list instances because pager fails to fetch page",
			mockAgentPoolList: func() []*armcontainerservice.AgentPool {
				return []*armcontainerservice.AgentPool{}
			},
			mockAgentPoolResp: func(apList []*armcontainerservice.AgentPool) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse] {
				return runtime.NewPager(runtime.PagingHandler[armcontainerservice.AgentPoolsClientListResponse]{
					More: func(page armcontainerservice.AgentPoolsClientListResponse) bool {
						return false
					},
					Fetcher: func(ctx context.Context, page *armcontainerservice.AgentPoolsClientListResponse) (armcontainerservice.AgentPoolsClientListResponse, error) {
						return armcontainerservice.AgentPoolsClientListResponse{}, errors.New("Failed to fetch page")
					},
				})
			},
			expectedError: errors.New("Failed to fetch page"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)
			if tc.mockAgentPoolResp != nil {
				pager := tc.mockAgentPoolResp(tc.mockAgentPoolList())
				agentPoolMocks.EXPECT().NewListPager(gomock.Any(), gomock.Any(), gomock.Any()).Return(pager)
			}

			mockK8sClient := fake.NewClient()
			if tc.callK8sMocks != nil {
				tc.callK8sMocks(mockK8sClient)
			}

			p := createTestProvider(agentPoolMocks, mockK8sClient)

			instanceList, err := p.List(context.Background())

			if tc.expectedError == nil {
				assert.NoError(t, err, "Not expected to return error")
				assert.NotNil(t, instanceList, "Response instance list should not be nil")
				assert.Equal(t, len(tc.mockAgentPoolList()), len(instanceList), "Number of Instances should be same as number of agent pools")

				for i := range tc.mockAgentPoolList() {
					assert.Equal(t, tc.mockAgentPoolList()[i].Name, instanceList[i].Name, "Instance name should be same as agent pool")
					assert.Equal(t, tc.mockAgentPoolList()[i].Properties.VMSize, instanceList[i].Type, "Instance type should be same as agent pool's vm size")
				}
			} else {
				assert.EqualError(t, err, tc.expectedError.Error())
			}
		})
	}
}

func TestFromAPListToInstanceFailure(t *testing.T) {
	testCases := []struct {
		name              string
		id                string
		mockAgentPoolList func(id string) []*armcontainerservice.AgentPool
		expectedError     func(err string) error
	}{
		{
			name: "Fail to get instance from agent pool list because no agentpools are found",
			mockAgentPoolList: func(id string) []*armcontainerservice.AgentPool {
				return []*armcontainerservice.AgentPool{}
			},
			expectedError: func(err string) error {
				return errors.New("nodeclaim not found, agentpools not found")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)
			mockK8sClient := fake.NewClient()

			p := createTestProvider(agentPoolMocks, mockK8sClient)

			instanceList, err := p.fromAPListToInstances(context.Background(), tc.mockAgentPoolList(tc.id))

			assert.EqualError(t, err, tc.expectedError(tc.id).Error())
			assert.Empty(t, instanceList, "Response instance list should be empty")
		})
	}
}

func TestCreateSuccess(t *testing.T) {
	testCases := []struct {
		name              string
		nodeClaim         *karpenterv1.NodeClaim
		mockAgentPoolResp func(nodeClaim *karpenterv1.NodeClaim, mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error)
		callK8sMocks      func(c *fake.MockClient)
	}{
		{
			name: "Successfully create instance",
			nodeClaim: fake.GetNodeClaimObj("agentpool0", map[string]string{"test": "test"}, []v1.Taint{},
				karpenterv1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI)),
				}},
				[]v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_NC6s_v3"},
					},
				}),
			mockAgentPoolResp: func(nodeClaim *karpenterv1.NodeClaim, mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error) {
				ap := GetAgentPoolObjWithName(nodeClaim.Name, "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", nodeClaim.Spec.Requirements[0].Values[0])

				createResp := armcontainerservice.AgentPoolsClientCreateOrUpdateResponse{
					AgentPool: ap,
				}
				resp := http.Response{StatusCode: http.StatusAccepted, Body: http.NoBody}

				mockHandler.EXPECT().Done().Return(true).Times(3)
				mockHandler.EXPECT().Result(gomock.Any(), gomock.Any()).Return(nil)

				pollingOptions := &runtime.NewPollerOptions[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]{
					Handler:  mockHandler,
					Response: &createResp,
				}

				p, err := runtime.NewPoller(&resp, runtime.NewPipeline("", "", runtime.PipelineOptions{}, nil), pollingOptions)
				return p, err
			},
			callK8sMocks: func(c *fake.MockClient) {
				nodeList := GetNodeList([]v1.Node{ReadyNode})
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
			name: "Successfully create instance after waiting for node to be ready",
			nodeClaim: fake.GetNodeClaimObj("agentpool0", map[string]string{"test": "test"}, []v1.Taint{},
				karpenterv1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI)),
				}},
				[]v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_NC6s_v3"},
					},
				}),
			mockAgentPoolResp: func(nodeClaim *karpenterv1.NodeClaim, mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error) {
				ap := GetAgentPoolObjWithName(nodeClaim.Name, "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", nodeClaim.Spec.Requirements[0].Values[0])

				createResp := armcontainerservice.AgentPoolsClientCreateOrUpdateResponse{
					AgentPool: ap,
				}
				resp := http.Response{StatusCode: http.StatusAccepted, Body: http.NoBody}

				mockHandler.EXPECT().Done().Return(true).Times(3)
				mockHandler.EXPECT().Result(gomock.Any(), gomock.Any()).Return(nil)

				pollingOptions := &runtime.NewPollerOptions[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]{
					Handler:  mockHandler,
					Response: &createResp,
				}

				p, err := runtime.NewPoller(&resp, runtime.NewPipeline("", "", runtime.PipelineOptions{}, nil), pollingOptions)
				return p, err
			},
			callK8sMocks: func(c *fake.MockClient) {
				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(nil).Once().Run(func(args mock.Arguments) {
					nodeList := GetNodeList([]v1.Node{ReadyNode})
					relevantMap := c.CreateMapWithType(nodeList)
					//insert node objects into the map
					for _, obj := range nodeList.Items {
						n := obj
						objKey := client.ObjectKeyFromObject(&n)

						relevantMap[objKey] = &n
					}
				})

				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(nil).Once()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)
			if tc.mockAgentPoolResp != nil {
				mockHandler := fake.NewMockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse](mockCtrl)

				p, err := tc.mockAgentPoolResp(tc.nodeClaim, mockHandler)
				agentPoolMocks.EXPECT().BeginCreateOrUpdate(gomock.Any(), gomock.Any(), gomock.Any(), tc.nodeClaim.Name, gomock.Any(), gomock.Any()).Return(p, err)
			}

			mockK8sClient := fake.NewClient()
			if tc.callK8sMocks != nil {
				tc.callK8sMocks(mockK8sClient)
			}

			p := createTestProvider(agentPoolMocks, mockK8sClient)

			instance, err := p.Create(context.Background(), tc.nodeClaim)

			assert.NoError(t, err, "Not expected to return error")
			assert.NotNil(t, instance, "Response instance should not be nil")
			assert.Equal(t, &tc.nodeClaim.Name, instance.Name, "Instance name should be same as nodeclaim name")
			assert.Equal(t, &tc.nodeClaim.Spec.Requirements[0].Values[0], instance.Type, "Instance type should be same as nodeclaim's instance type")
		})
	}
}

func TestCreateFailure(t *testing.T) {
	testCases := []struct {
		name              string
		nodeClaim         *karpenterv1.NodeClaim
		mockAgentPoolResp func(nodeClaim *karpenterv1.NodeClaim, mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error)
		callK8sMocks      func(c *fake.MockClient)
		expectedError     error
	}{
		{
			name: "Fail to create instance because node is not found and returns error on retry",
			nodeClaim: fake.GetNodeClaimObj("agentpool0", map[string]string{"test": "test"}, []v1.Taint{},
				karpenterv1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI)),
				}},
				[]v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_NC6s_v3"},
					},
				}),
			mockAgentPoolResp: func(nodeClaim *karpenterv1.NodeClaim, mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error) {
				ap := GetAgentPoolObjWithName(nodeClaim.Name, "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", nodeClaim.Spec.Requirements[0].Values[0])

				createResp := armcontainerservice.AgentPoolsClientCreateOrUpdateResponse{
					AgentPool: ap,
				}
				resp := http.Response{StatusCode: http.StatusAccepted, Body: http.NoBody}

				mockHandler.EXPECT().Done().Return(true).Times(3)
				mockHandler.EXPECT().Result(gomock.Any(), gomock.Any()).Return(nil)

				pollingOptions := &runtime.NewPollerOptions[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]{
					Handler:  mockHandler,
					Response: &createResp,
				}

				p, err := runtime.NewPoller(&resp, runtime.NewPipeline("", "", runtime.PipelineOptions{}, nil), pollingOptions)
				return p, err
			},
			callK8sMocks: func(c *fake.MockClient) {
				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(nil).Once()

				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(errors.New("fail to find the node object"))
			},
			expectedError: errors.New("fail to find the node object"),
		},
		{
			name: "Fail to create instance because node object is not found",
			nodeClaim: fake.GetNodeClaimObj("agentpool0", map[string]string{"test": "test"}, []v1.Taint{},
				karpenterv1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI)),
				}},
				[]v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_NC6s_v3"},
					},
				}),
			mockAgentPoolResp: func(nodeClaim *karpenterv1.NodeClaim, mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error) {
				ap := GetAgentPoolObjWithName(nodeClaim.Name, "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss", nodeClaim.Spec.Requirements[0].Values[0])

				createResp := armcontainerservice.AgentPoolsClientCreateOrUpdateResponse{
					AgentPool: ap,
				}
				resp := http.Response{StatusCode: http.StatusAccepted, Body: http.NoBody}

				mockHandler.EXPECT().Done().Return(true).Times(3)
				mockHandler.EXPECT().Result(gomock.Any(), gomock.Any()).Return(nil)

				pollingOptions := &runtime.NewPollerOptions[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]{
					Handler:  mockHandler,
					Response: &createResp,
				}

				p, err := runtime.NewPoller(&resp, runtime.NewPipeline("", "", runtime.PipelineOptions{}, nil), pollingOptions)
				return p, err
			},
			callK8sMocks: func(c *fake.MockClient) {
				c.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(nil)
			},
			expectedError: errors.New("fail to find the node object"),
		},
		{
			name: "Fail to delete instance because poller returns error",
			nodeClaim: fake.GetNodeClaimObj("agentpool0", map[string]string{"test": "test"}, []v1.Taint{},
				karpenterv1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI)),
				}},
				[]v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_NC6s_v3"},
					},
				}),
			mockAgentPoolResp: func(nodeClaim *karpenterv1.NodeClaim, mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error) {
				createResp := armcontainerservice.AgentPoolsClientCreateOrUpdateResponse{
					AgentPool: armcontainerservice.AgentPool{},
				}
				resp := http.Response{StatusCode: http.StatusBadRequest, Body: http.NoBody}

				mockHandler.EXPECT().Done().Return(false)
				mockHandler.EXPECT().Poll(gomock.Any()).Return(&resp, errors.New("Failed to fetch latest status of operation"))

				pollingOptions := &runtime.NewPollerOptions[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]{
					Handler:  mockHandler,
					Response: &createResp,
				}

				p, err := runtime.NewPoller(&resp, runtime.NewPipeline("", "", runtime.PipelineOptions{}, nil), pollingOptions)
				return p, err
			},
			expectedError: errors.New("Failed to fetch latest status of operation"),
		},
		{
			name: "Fail to create instance because agentPool.CreateOrUpdate returns a failure",
			nodeClaim: fake.GetNodeClaimObj("agentpool0", map[string]string{"test": "test"}, []v1.Taint{},
				karpenterv1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI)),
				}},
				[]v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_D4s_v4"},
					},
				}),
			mockAgentPoolResp: func(nodeClaim *karpenterv1.NodeClaim, mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error) {
				return nil, errors.New("Failed to create agent pool")
			},
			expectedError: errors.New("Failed to create agent pool"),
		},
		{
			name: "Fail to create instance because nodeClaim spec does not have requirement for instance type",
			nodeClaim: fake.GetNodeClaimObj("agentpool000", map[string]string{"test": "test"}, []v1.Taint{},
				karpenterv1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI)),
				}},
				[]v1.NodeSelectorRequirement{}),
			expectedError: errors.New("nodeClaim spec has no requirement for instance type"),
		},
		{
			name: "Fail to create instance because of invalid nodeClaim name",
			nodeClaim: fake.GetNodeClaimObj("invalid-name", map[string]string{"test": "test"}, []v1.Taint{},
				karpenterv1.ResourceRequirements{Requests: v1.ResourceList{
					v1.ResourceStorage: lo.FromPtr(resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI)),
				}},
				[]v1.NodeSelectorRequirement{}),
			expectedError: errors.New("is invalid, must match regex pattern: ^[a-z][a-z0-9]{0,11}$"),
		},
		{
			name: "Fail to create instance because of no storage request",
			nodeClaim: fake.GetNodeClaimObj("agentpool000", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
				{
					Key:      "node.kubernetes.io/instance-type",
					Operator: "In",
					Values:   []string{"Standard_D4s_v4"},
				},
			}),
			expectedError: errors.New("storage request of nodeclaim(agentpool000) should be more than 0"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)
			if tc.mockAgentPoolResp != nil {
				mockHandler := fake.NewMockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse](mockCtrl)

				p, err := tc.mockAgentPoolResp(tc.nodeClaim, mockHandler)
				agentPoolMocks.EXPECT().BeginCreateOrUpdate(gomock.Any(), gomock.Any(), gomock.Any(), tc.nodeClaim.Name, gomock.Any(), gomock.Any()).Return(p, err)
			}

			mockK8sClient := fake.NewClient()
			if tc.callK8sMocks != nil {
				tc.callK8sMocks(mockK8sClient)
			}

			p := createTestProvider(agentPoolMocks, mockK8sClient)

			instance, err := p.Create(context.Background(), tc.nodeClaim)

			assert.Contains(t, err.Error(), tc.expectedError.Error())
			assert.Nil(t, instance, "Response instance should be nil")
		})
	}
}

func TestDetermineOSSKUWithNilNodeClaim(t *testing.T) {
	result := determineOSSKU(nil)
	assert.Equal(t, armcontainerservice.OSSKUUbuntu, *result)
}

func TestNewAgentPoolObjectWithImageFamily(t *testing.T) {
	testCases := []struct {
		name          string
		vmSize        string
		nodeClaim     *karpenterv1.NodeClaim
		expectedOSSKU armcontainerservice.OSSKU
	}{
		{
			name:   "NodeClaim with AzureLinux image family label",
			vmSize: "Standard_NC6s_v3",
			nodeClaim: &karpenterv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nodeclaim",
					Labels: map[string]string{
						"kaito.sh/node-image-family": "AzureLinux",
					},
				},
				Spec: karpenterv1.NodeClaimSpec{
					Resources: karpenterv1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: *resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI),
						},
					},
				},
			},
			expectedOSSKU: armcontainerservice.OSSKUAzureLinux,
		},
		{
			name:   "NodeClaim with Ubuntu image family label",
			vmSize: "Standard_NC6s_v3",
			nodeClaim: &karpenterv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nodeclaim",
					Labels: map[string]string{
						"kaito.sh/node-image-family": "Ubuntu",
					},
				},
				Spec: karpenterv1.NodeClaimSpec{
					Resources: karpenterv1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: *resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI),
						},
					},
				},
			},
			expectedOSSKU: armcontainerservice.OSSKUUbuntu,
		},
		{
			name:   "NodeClaim with Ubuntu2204 image family label",
			vmSize: "Standard_NC6s_v3",
			nodeClaim: &karpenterv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nodeclaim",
					Labels: map[string]string{
						"kaito.sh/node-image-family": "Ubuntu2204",
					},
				},
				Spec: karpenterv1.NodeClaimSpec{
					Resources: karpenterv1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: *resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI),
						},
					},
				},
			},
			expectedOSSKU: armcontainerservice.OSSKUUbuntu,
		},
		{
			name:   "NodeClaim with AzureLinux image family annotation",
			vmSize: "Standard_NC6s_v3",
			nodeClaim: &karpenterv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nodeclaim",
					Annotations: map[string]string{
						"kaito.sh/node-image-family": "AzureLinux",
					},
				},
				Spec: karpenterv1.NodeClaimSpec{
					Resources: karpenterv1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: *resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI),
						},
					},
				},
			},
			expectedOSSKU: armcontainerservice.OSSKUAzureLinux,
		},
		{
			name:   "NodeClaim with case-insensitive AzureLinux image family label",
			vmSize: "Standard_NC6s_v3",
			nodeClaim: &karpenterv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nodeclaim",
					Labels: map[string]string{
						"kaito.sh/node-image-family": "azurelinux",
					},
				},
				Spec: karpenterv1.NodeClaimSpec{
					Resources: karpenterv1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: *resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI),
						},
					},
				},
			},
			expectedOSSKU: armcontainerservice.OSSKUAzureLinux,
		},
		{
			name:   "NodeClaim with unknown image family defaults to Ubuntu",
			vmSize: "Standard_NC6s_v3",
			nodeClaim: &karpenterv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nodeclaim",
					Labels: map[string]string{
						"kaito.sh/node-image-family": "Unknown",
					},
				},
				Spec: karpenterv1.NodeClaimSpec{
					Resources: karpenterv1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: *resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI),
						},
					},
				},
			},
			expectedOSSKU: armcontainerservice.OSSKUUbuntu,
		},
		{
			name:   "NodeClaim without image family label defaults to Ubuntu",
			vmSize: "Standard_NC6s_v3",
			nodeClaim: &karpenterv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nodeclaim",
				},
				Spec: karpenterv1.NodeClaimSpec{
					Resources: karpenterv1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: *resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI),
						},
					},
				},
			},
			expectedOSSKU: armcontainerservice.OSSKUUbuntu,
		},
		{
			name:   "Label takes precedence over annotation",
			vmSize: "Standard_NC6s_v3",
			nodeClaim: &karpenterv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nodeclaim",
					Labels: map[string]string{
						"kaito.sh/node-image-family": "AzureLinux",
					},
					Annotations: map[string]string{
						"kaito.sh/node-image-family": "Ubuntu",
					},
				},
				Spec: karpenterv1.NodeClaimSpec{
					Resources: karpenterv1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: *resource.NewQuantity(30*1024*1024*1024, resource.DecimalSI),
						},
					},
				},
			},
			expectedOSSKU: armcontainerservice.OSSKUAzureLinux,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := newAgentPoolObject(tc.vmSize, tc.nodeClaim)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedOSSKU, *result.Properties.OSSKU)
			assert.Equal(t, armcontainerservice.OSTypeLinux, *result.Properties.OSType)
		})
	}
}

func createTestProvider(agentPoolsAPIMocks *fake.MockAgentPoolsAPI, mockK8sClient *fake.MockClient) *Provider {
	mockAzClient := NewAZClientFromAPI(agentPoolsAPIMocks)
	return NewProvider(mockAzClient, mockK8sClient, "testRG", "testCluster")
}

func GetAgentPoolObj(apType armcontainerservice.AgentPoolType, capacityType armcontainerservice.ScaleSetPriority,
	labels map[string]*string, taints []*string, diskSizeGB int32, vmSize string) armcontainerservice.AgentPool {
	return armcontainerservice.AgentPool{
		Properties: &armcontainerservice.ManagedClusterAgentPoolProfileProperties{
			NodeLabels:       labels,
			NodeTaints:       taints,
			Type:             lo.ToPtr(apType),
			VMSize:           lo.ToPtr(vmSize),
			OSType:           lo.ToPtr(armcontainerservice.OSTypeLinux),
			Count:            lo.ToPtr(int32(1)),
			ScaleSetPriority: lo.ToPtr(capacityType),
			OSDiskSizeGB:     lo.ToPtr(diskSizeGB),
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
				"test":                       lo.ToPtr("test"),
				"kaito.sh/workspace":         lo.ToPtr("none"),
				karpenterv1.NodePoolLabelKey: lo.ToPtr("kaito"),
			},
			ProvisioningState: lo.ToPtr("Succeeded"),
			PowerState: &armcontainerservice.PowerState{
				Code: lo.ToPtr(armcontainerservice.CodeRunning),
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
		ObjectMeta: metav1.ObjectMeta{
			Name: "aks-agentpool0-20562481-vmss_0",
			Labels: map[string]string{
				"agentpool":                      "agentpool0",
				"kubernetes.azure.com/agentpool": "agentpool0",
			},
		},
		Spec: v1.NodeSpec{
			ProviderID: "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-agentpool0-20562481-vmss/virtualMachines/0",
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
