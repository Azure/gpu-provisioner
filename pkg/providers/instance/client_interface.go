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

	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// ClusterType represents the type of cluster (AKS or Arc)
type ClusterType string

const (
	ClusterTypeAKS ClusterType = "aks"
	ClusterTypeArc ClusterType = "arc"
)

// AgentPoolParams contains the parameters needed for agent pool operations
type AgentPoolParams struct {
	SubscriptionID string
	ResourceGroup  string
	ClusterName    string
	AgentPoolName  string
	NodeClaim      *karpenterv1.NodeClaim // Direct NodeClaim instead of AgentPoolSpec
	VMSize         string                 // Extracted from NodeClaim requirements
}

// AgentPoolInfo contains the common information about an agent pool
type AgentPoolInfo struct {
	Name              *string
	ID                *string
	ProvisioningState interface{} // Can be different types for AKS vs Arc
	VMSize            *string
	Count             *int32
	NodeLabels        map[string]*string
	Tags              map[string]*string
	VnetSubnetID      *string
	NodeImageVersion  *string
}

// AgentPoolClient defines the common interface for both AKS and Arc agent pool operations
type AgentPoolClient interface {
	CreateOrUpdate(ctx context.Context, params AgentPoolParams) (*AgentPoolInfo, error)
	Get(ctx context.Context, params AgentPoolParams) (*AgentPoolInfo, error)
	Delete(ctx context.Context, params AgentPoolParams) error
	List(ctx context.Context, params AgentPoolParams) ([]*AgentPoolInfo, error)
}

// PollerWrapper wraps different poller types to provide a common interface
type PollerWrapper interface {
	PollUntilDone(ctx context.Context, options interface{}) (interface{}, error)
}
