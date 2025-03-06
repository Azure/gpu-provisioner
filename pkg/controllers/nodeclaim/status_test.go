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

package nodeclaim

import (
	"context"
	"fmt"
	"testing"

	"github.com/azure/gpu-provisioner/pkg/fake"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func TestReconcile(t *testing.T) {
	testcases := map[string]struct {
		nodeClaim           *karpenterv1.NodeClaim
		initNodeReadyStatus bool
		node                *v1.Node
		expectedReadyStatus metav1.ConditionStatus
		expectedError       error
	}{
		"nodeclaim status change from true to false": {
			nodeClaim: fake.GetNodeClaimObj("agentpool1", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
				{
					Key:      "node.kubernetes.io/instance-type",
					Operator: "In",
					Values:   []string{"Standard_NC6s_v3"},
				},
			}),
			initNodeReadyStatus: true,
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("aks-%s-20562481-vmss_0", "agentpool1"),
					Labels: map[string]string{
						"agentpool":                      "agentpool1",
						"kubernetes.azure.com/agentpool": "agentpool1",
						karpenterv1.NodePoolLabelKey:     "kaito",
					},
				},
				Spec: v1.NodeSpec{
					ProviderID: fmt.Sprintf("azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-%s-20562481-vmss/virtualMachines/0", "agentpool1"),
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionFalse,
						},
					},
				},
			},
			expectedReadyStatus: metav1.ConditionStatus(v1.ConditionFalse),
			expectedError:       nil,
		},
		"nodeclaim status change from false to true": {
			nodeClaim: fake.GetNodeClaimObj("agentpool1", map[string]string{"test": "test"}, []v1.Taint{}, karpenterv1.ResourceRequirements{}, []v1.NodeSelectorRequirement{
				{
					Key:      "node.kubernetes.io/instance-type",
					Operator: "In",
					Values:   []string{"Standard_NC6s_v3"},
				},
			}),
			initNodeReadyStatus: false,
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("aks-%s-20562481-vmss_0", "agentpool1"),
					Labels: map[string]string{
						"agentpool":                      "agentpool1",
						"kubernetes.azure.com/agentpool": "agentpool1",
						karpenterv1.NodePoolLabelKey:     "kaito",
					},
				},
				Spec: v1.NodeSpec{
					ProviderID: fmt.Sprintf("azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/nodeRG/providers/Microsoft.Compute/virtualMachineScaleSets/aks-%s-20562481-vmss/virtualMachines/0", "agentpool1"),
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			},
			expectedReadyStatus: metav1.ConditionStatus(v1.ConditionTrue),
			expectedError:       nil,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			// init nodeclaim
			tc.nodeClaim.StatusConditions().SetTrue(karpenterv1.ConditionTypeLaunched)
			tc.nodeClaim.StatusConditions().SetTrue(karpenterv1.ConditionTypeRegistered)
			tc.nodeClaim.StatusConditions().SetTrue(karpenterv1.ConditionTypeInitialized)
			if tc.initNodeReadyStatus {
				tc.nodeClaim.StatusConditions().SetTrue(karpenterv1.ConditionTypeNodeReady)
			} else {
				tc.nodeClaim.StatusConditions().SetFalse(karpenterv1.ConditionTypeNodeReady, "init", "init false")
			}

			// prepare kubeclient
			// sigs.k8s.io/karpenter/pkg/apis/v1/doc.go
			// karpenter scheme has been registered in scheme.Scheme
			fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme.Scheme).
				WithStatusSubresource(&karpenterv1.NodeClaim{}).
				WithRuntimeObjects(tc.node).
				WithRuntimeObjects(tc.nodeClaim).
				WithIndex(&karpenterv1.NodeClaim{}, "status.providerID", func(o client.Object) []string {
					return []string{o.(*karpenterv1.NodeClaim).Status.ProviderID}
				}).
				Build()

			// create nodeclaim status controller
			c := NewController(fakeClient)
			_, err := c.Reconcile(context.Background(), tc.node)

			if tc.expectedError != nil {
				assert.Contains(t, err.Error(), tc.expectedError.Error())
			} else {
				assert.NoError(t, err, "expect no error but got one")

				var nc karpenterv1.NodeClaim
				err = c.kubeClient.Get(context.Background(), client.ObjectKeyFromObject(tc.nodeClaim), &nc)
				assert.NoError(t, err, "expect get current nodeclaim")
				assert.Equal(t, tc.expectedReadyStatus, nc.StatusConditions().Root().Status, "ready condition status is not equal")
			}
		})
	}
}
