/*
Copyright The Kubernetes Authors.

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

package garbagecollection

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/azure/gpu-provisioner/pkg/cloudprovider"
	"github.com/azure/gpu-provisioner/pkg/fake"
	"github.com/azure/gpu-provisioner/pkg/providers/instance"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func TestReconcile(t *testing.T) {
	testcases := map[string]struct {
		nodeClaims              []*karpenterv1.NodeClaim
		leakedNodeClaims        []*karpenterv1.NodeClaim
		mockListAgentPoolResp   func(nodeClaims []*karpenterv1.NodeClaim) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse]
		mockDeleteAgentPoolResp func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error)
		expectedError           error
	}{
		"garbage collection leaked instance without providerID successfully": {
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
			leakedNodeClaims: []*karpenterv1.NodeClaim{
				fake.GetNodeClaimObjWithoutProviderID("agentpool3", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_NC6s_v3"},
					},
				}),
			},
			mockListAgentPoolResp: func(nodeClaims []*karpenterv1.NodeClaim) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse] {
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
			mockDeleteAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
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
		"garbage collection leaked instance with providerID successfully": {
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
			leakedNodeClaims: []*karpenterv1.NodeClaim{
				fake.GetNodeClaimObj("agentpool3", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_NC6s_v3"},
					},
				}),
			},
			mockListAgentPoolResp: func(nodeClaims []*karpenterv1.NodeClaim) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse] {
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
			mockDeleteAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
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
		"failed to garbage collection leaked instance": {
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
			leakedNodeClaims: []*karpenterv1.NodeClaim{
				fake.GetNodeClaimObjWithoutProviderID("agentpool3", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
					{
						Key:      "node.kubernetes.io/instance-type",
						Operator: "In",
						Values:   []string{"Standard_NC6s_v3"},
					},
				}),
			},
			mockListAgentPoolResp: func(nodeClaims []*karpenterv1.NodeClaim) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse] {
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
			mockDeleteAgentPoolResp: func(mockHandler *fake.MockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse]) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error) {
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
			if tc.mockListAgentPoolResp != nil {
				pager := tc.mockListAgentPoolResp(append(tc.nodeClaims, tc.leakedNodeClaims...))
				agentPoolMocks.EXPECT().NewListPager(gomock.Any(), gomock.Any(), gomock.Any()).Return(pager)
			}

			if tc.mockDeleteAgentPoolResp != nil {
				mockHandler := fake.NewMockPollingHandler[armcontainerservice.AgentPoolsClientDeleteResponse](mockCtrl)
				resp, err := tc.mockDeleteAgentPoolResp(mockHandler)
				agentPoolMocks.EXPECT().BeginDelete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, err)
			}

			// prepare kubeclient
			// sigs.k8s.io/karpenter/pkg/apis/v1/doc.go
			// karpenter scheme has been registered in scheme.Scheme
			nodeList := fake.CreateNodeListWithNodeClaim(append(tc.nodeClaims, tc.leakedNodeClaims...))
			nodes := lo.FilterMap(nodeList.Items, func(node v1.Node, _ int) (k8sruntime.Object, bool) {
				return &node, true
			})

			nodeClaims := lo.FilterMap(tc.nodeClaims, func(nc *karpenterv1.NodeClaim, _ int) (k8sruntime.Object, bool) {
				return nc, true
			})

			fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme.Scheme).
				WithRuntimeObjects(nodes...).
				WithRuntimeObjects(nodeClaims...).
				WithIndex(&v1.Node{}, "spec.providerID", func(o client.Object) []string {
					return []string{o.(*v1.Node).Spec.ProviderID}
				}).
				Build()

			// prepare instance provider
			mockAzClient := instance.NewAZClientFromAPI(agentPoolMocks)
			instanceProvider := instance.NewProvider(mockAzClient, fakeClient, "testRG", "testCluster")

			// create cloud provider
			cloudProvider := cloudprovider.New(instanceProvider, nil)

			// create garbage collection controller
			c := NewController(fakeClient, cloudProvider)
			_, err := c.Reconcile(context.Background())

			if tc.expectedError != nil {
				assert.Contains(t, err.Error(), tc.expectedError.Error())
			} else {
				assert.NoError(t, err, "expect no error but got one")
			}
		})
	}
}
