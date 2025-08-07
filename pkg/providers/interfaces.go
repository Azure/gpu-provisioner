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

package providers

import (
	"context"

	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (
	LabelMachineType       = "kaito.sh/machine-type"
	NodeClaimCreationLabel = "kaito.sh/creation-timestamp"
	// use self-defined layout in order to satisfy node label syntax
	CreationTimestampLayout = "2006-01-02T15-04-05Z"
)

// InstanceProvider defines the interface that all instance providers must implement
// This allows CloudProvider to work with different provider implementations (AKS, Arc, etc.)
// Since both AKS and Arc use the same instance.Instance type, we can simplify the interface
type InstanceProvider interface {
	// Create creates a new instance based on the NodeClaim requirements
	Create(ctx context.Context, nodeClaim *karpenterv1.NodeClaim) (*Instance, error)

	// Get retrieves an instance by its provider ID
	Get(ctx context.Context, providerID string) (*Instance, error)

	// List retrieves all instances managed by this provider
	List(ctx context.Context) ([]*Instance, error)

	// Delete removes an instance by its provider ID
	Delete(ctx context.Context, providerID string) error
}

type Instance struct {
	Name         *string // agentPoolName or instance/vmName
	State        *string
	ID           *string
	ImageID      *string
	Type         *string
	CapacityType *string
	SubnetID     *string
	Tags         map[string]*string
	Labels       map[string]string
}
