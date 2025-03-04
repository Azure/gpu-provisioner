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

package cloudprovider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/azure/gpu-provisioner/pkg/fake"
	"github.com/azure/gpu-provisioner/pkg/providers/instance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

func TestCreate(t *testing.T) {
	testcases := map[string]struct {
		nodeClaim         *karpenterv1.NodeClaim
		mockAgentPoolResp func(nodeClaim *karpenterv1.NodeClaim, mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error)
		expectedError     bool
	}{
		"successfully create instance": {
			nodeClaim: fake.GetNodeClaimObj("agentpool0", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
				{
					Key:      "node.kubernetes.io/instance-type",
					Operator: "In",
					Values:   []string{"Standard_NC6s_v3"},
				},
			}),
			mockAgentPoolResp: func(nodeClaim *karpenterv1.NodeClaim, mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error) {
				ap := fake.CreateAgentPoolObjWithNodeClaim(nodeClaim)

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
			expectedError: false,
		},
		"failed to create instance": {
			nodeClaim: fake.GetNodeClaimObj("invalid-nodeclaim-name", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
				{
					Key:      "node.kubernetes.io/instance-type",
					Operator: "In",
					Values:   []string{"Standard_NC6s_v3"},
				},
			}),
			expectedError: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// prepare agentPoolClient with poller
			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)
			if tc.mockAgentPoolResp != nil {
				mockHandler := fake.NewMockPollingHandler[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse](mockCtrl)
				p, err := tc.mockAgentPoolResp(tc.nodeClaim, mockHandler)
				agentPoolMocks.EXPECT().BeginCreateOrUpdate(gomock.Any(), gomock.Any(), gomock.Any(), tc.nodeClaim.Name, gomock.Any(), gomock.Any()).Return(p, err)
			}

			// prepare kubeclient
			mockK8sClient := fake.NewClient()
			nodeList := fake.CreateNodeListWithNodeClaim([]*karpenterv1.NodeClaim{tc.nodeClaim})
			relevantMap := mockK8sClient.CreateMapWithType(nodeList)
			for _, obj := range nodeList.Items {
				n := obj
				objKey := client.ObjectKeyFromObject(&n)
				relevantMap[objKey] = &n
			}
			mockK8sClient.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(nil)

			// prepare instance provider
			mockAzClient := instance.NewAZClientFromAPI(agentPoolMocks)
			instanceProvider := instance.NewProvider(mockAzClient, mockK8sClient, "testRG", "testCluster")

			// create cloud provider and call create function
			cloudProvider := New(instanceProvider, nil)
			nc, err := cloudProvider.Create(context.Background(), tc.nodeClaim)

			if tc.expectedError {
				assert.Error(t, err, "expect error but got nil")
			} else if !tc.expectedError {
				assert.NoError(t, err, "Not expected to return error")
			}

			if nc != nil {
				assert.Equal(t, nc.Name, tc.nodeClaim.Name, "nodeclaim name is not the same")
				assert.NotEmpty(t, nc.Status.ProviderID, "provider id is not empty")
			}
		})
	}
}

func TestList(t *testing.T) {
	testcases := map[string]struct {
		nodeClaims        []*karpenterv1.NodeClaim
		mockAgentPoolResp func(nodeClaims []*karpenterv1.NodeClaim) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse]
		expectedError     bool
	}{
		"successfully list instances": {
			nodeClaims: []*karpenterv1.NodeClaim{
				fake.GetNodeClaimObj("agentpool1", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_NC6s_v3"},
					},
				}),
				fake.GetNodeClaimObj("agentpool2", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_NC6s_v3"},
					},
				}),
			},
			mockAgentPoolResp: func(nodeClaims []*karpenterv1.NodeClaim) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse] {
				var agentPools []*armcontainerservice.AgentPool
				for i := range nodeClaims {
					ap := fake.CreateAgentPoolObjWithNodeClaim(nodeClaims[i])
					agentPools = append(agentPools, &ap)
				}
				return runtime.NewPager(runtime.PagingHandler[armcontainerservice.AgentPoolsClientListResponse]{
					More: func(page armcontainerservice.AgentPoolsClientListResponse) bool {
						return false
					},
					Fetcher: func(ctx context.Context, page *armcontainerservice.AgentPoolsClientListResponse) (armcontainerservice.AgentPoolsClientListResponse, error) {
						return armcontainerservice.AgentPoolsClientListResponse{
							AgentPoolListResult: armcontainerservice.AgentPoolListResult{
								Value: agentPools,
							},
						}, nil
					},
				})
			},
			expectedError: false,
		},
		"failed to list instances": {
			mockAgentPoolResp: func(nodeClaims []*karpenterv1.NodeClaim) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse] {
				return runtime.NewPager(runtime.PagingHandler[armcontainerservice.AgentPoolsClientListResponse]{
					More: func(page armcontainerservice.AgentPoolsClientListResponse) bool {
						return false
					},
					Fetcher: func(ctx context.Context, page *armcontainerservice.AgentPoolsClientListResponse) (armcontainerservice.AgentPoolsClientListResponse, error) {
						return armcontainerservice.AgentPoolsClientListResponse{}, errors.New("Failed to fetch page")
					},
				})
			},
			expectedError: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// prepare agentPoolClient with poller
			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)
			if tc.mockAgentPoolResp != nil {
				pager := tc.mockAgentPoolResp(tc.nodeClaims)
				agentPoolMocks.EXPECT().NewListPager(gomock.Any(), gomock.Any(), gomock.Any()).Return(pager)
			}

			// prepare kubeclient
			mockK8sClient := fake.NewClient()
			mockK8sClient.On("List", mock.IsType(context.Background()), mock.IsType(&v1.NodeList{}), mock.Anything).Return(nil)

			// prepare instance provider
			mockAzClient := instance.NewAZClientFromAPI(agentPoolMocks)
			instanceProvider := instance.NewProvider(mockAzClient, mockK8sClient, "testRG", "testCluster")

			// create cloud provider and call list function
			cloudProvider := New(instanceProvider, nil)
			nodeClaims, err := cloudProvider.List(context.Background())

			if tc.expectedError {
				assert.Error(t, err, "expect error but got nil")
			} else if !tc.expectedError {
				assert.NoError(t, err, "Not expected to return error")
			}

			assert.Equal(t, len(tc.nodeClaims), len(nodeClaims), "Number of NodeClaims should be same")
			for i := range tc.nodeClaims {
				assert.Equal(t, nodeClaims[i].Name, tc.nodeClaims[i].Name, "NodeClaim name should be same")
				assert.Equal(t, nodeClaims[i].Labels[v1.LabelInstanceTypeStable], tc.nodeClaims[i].Spec.Requirements[0].Values[0], "Instance type should be same")
			}
		})
	}
}

func TestGet(t *testing.T) {
	testcases := map[string]struct {
		nodeClaim                *karpenterv1.NodeClaim
		mockAgentPoolResp        func(nodeClaim *karpenterv1.NodeClaim) (armcontainerservice.AgentPoolsClientGetResponse, error)
		expectedError            error
		IsNodeClaimNotFoundError bool
	}{
		"successfully get instance": {
			nodeClaim: fake.GetNodeClaimObj("agentpool1", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
				{
					Key:      "node.kubernetes.io/instance-type",
					Operator: "In",
					Values:   []string{"Standard_NC6s_v3"},
				},
			}),
			mockAgentPoolResp: func(nodeClaim *karpenterv1.NodeClaim) (armcontainerservice.AgentPoolsClientGetResponse, error) {
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: fake.CreateAgentPoolObjWithNodeClaim(nodeClaim)}, nil
			},
			expectedError: nil,
		},
		"failed to get instance": {
			nodeClaim: fake.GetNodeClaimObj("agentpool1", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
				{
					Key:      "node.kubernetes.io/instance-type",
					Operator: "In",
					Values:   []string{"Standard_NC6s_v3"},
				},
			}),
			mockAgentPoolResp: func(nodeClaim *karpenterv1.NodeClaim) (armcontainerservice.AgentPoolsClientGetResponse, error) {
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: fake.CreateAgentPoolObjWithNodeClaim(nodeClaim)}, fmt.Errorf("internal server error")
			},
			expectedError: errors.New("internal server error"),
		},
		"instance doesn't exist": {
			nodeClaim: fake.GetNodeClaimObj("agentpool1", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
				{
					Key:      "node.kubernetes.io/instance-type",
					Operator: "In",
					Values:   []string{"Standard_NC6s_v3"},
				},
			}),
			mockAgentPoolResp: func(nodeClaim *karpenterv1.NodeClaim) (armcontainerservice.AgentPoolsClientGetResponse, error) {
				return armcontainerservice.AgentPoolsClientGetResponse{AgentPool: fake.CreateAgentPoolObjWithNodeClaim(nodeClaim)}, fmt.Errorf("Agent Pool not found")
			},
			IsNodeClaimNotFoundError: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// prepare agentPoolClient with poller
			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)
			if tc.mockAgentPoolResp != nil {
				resp, err := tc.mockAgentPoolResp(tc.nodeClaim)
				agentPoolMocks.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), tc.nodeClaim.Name, gomock.Any()).Return(resp, err)
			}

			// prepare instance provider
			mockAzClient := instance.NewAZClientFromAPI(agentPoolMocks)
			instanceProvider := instance.NewProvider(mockAzClient, nil, "testRG", "testCluster")

			// create cloud provider and call list function
			cloudProvider := New(instanceProvider, nil)
			nodeClaim, err := cloudProvider.Get(context.Background(), tc.nodeClaim.Status.ProviderID)

			if tc.IsNodeClaimNotFoundError {
				if !cloudprovider.IsNodeClaimNotFoundError(err) {
					assert.Error(t, err, "expect IsNodeClaimNotFoundError but got other error")
				}
			} else if tc.expectedError != nil {
				assert.Contains(t, err.Error(), tc.expectedError.Error())
			} else {
				assert.Equal(t, nodeClaim.Name, tc.nodeClaim.Name, "NodeClaim name should be same")
				assert.Equal(t, nodeClaim.Status.ProviderID, tc.nodeClaim.Status.ProviderID, "Instance ProviderID should be same")
			}
		})
	}
}

func TestDelete(t *testing.T) {
	testcases := map[string]struct {
		nodeClaim         *karpenterv1.NodeClaim
		mockAgentPoolResp func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error)
		expectedError     error
	}{
		"successfully delete instance": {
			nodeClaim: fake.GetNodeClaimObj("agentpool1", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
				{
					Key:      "node.kubernetes.io/instance-type",
					Operator: "In",
					Values:   []string{"Standard_NC6s_v3"},
				},
			}),
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
			expectedError: nil,
		},
		"failed to delete instance": {
			nodeClaim: fake.GetNodeClaimObj("agentpool1", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
				{
					Key:      "node.kubernetes.io/instance-type",
					Operator: "In",
					Values:   []string{"Standard_NC6s_v3"},
				},
			}),
			mockAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
				return nil, errors.New("internal server error")
			},
			expectedError: errors.New("internal server error"),
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// prepare agentPoolClient with poller
			agentPoolMocks := fake.NewMockAgentPoolsAPI(mockCtrl)
			if tc.mockAgentPoolResp != nil {
				mockHandler := fake.NewMockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse](mockCtrl)
				resp, err := tc.mockAgentPoolResp(mockHandler)
				agentPoolMocks.EXPECT().BeginDelete(gomock.Any(), gomock.Any(), gomock.Any(), tc.nodeClaim.Name, gomock.Any()).Return(resp, err)
			}

			// prepare instance provider
			mockAzClient := instance.NewAZClientFromAPI(agentPoolMocks)
			instanceProvider := instance.NewProvider(mockAzClient, nil, "testRG", "testCluster")

			// create cloud provider and call list function
			cloudProvider := New(instanceProvider, nil)
			err := cloudProvider.Delete(context.Background(), tc.nodeClaim)

			if tc.expectedError != nil {
				assert.Contains(t, err.Error(), tc.expectedError.Error())
			} else {
				assert.NoError(t, err, "expect no error but got one")
			}
		})
	}
}
