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

package common

import (
	"context"

	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/azure/gpu-provisioner/pkg/providers/instance"
)

// InstanceProvider defines the interface that all instance providers must implement
// This allows CloudProvider to work with different provider implementations (AKS, Arc, etc.)
// Since both AKS and Arc use the same instance.Instance type, we can simplify the interface
type InstanceProvider interface {
	// Create creates a new instance based on the NodeClaim requirements
	Create(ctx context.Context, nodeClaim *karpenterv1.NodeClaim) (*instance.Instance, error)

	// Get retrieves an instance by its provider ID
	Get(ctx context.Context, providerID string) (*instance.Instance, error)

	// List retrieves all instances managed by this provider
	List(ctx context.Context) ([]*instance.Instance, error)

	// Delete removes an instance by its provider ID
	Delete(ctx context.Context, providerID string) error
}
