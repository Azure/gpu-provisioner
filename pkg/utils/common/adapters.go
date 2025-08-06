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

	"github.com/azure/gpu-provisioner/pkg/providers/instance"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// AKSInstanceProviderAdapter adapts the AKS instance.Provider to implement common.InstanceProvider

type AKSInstanceProviderAdapter struct {
	provider *instance.Provider
}

// NewAKSInstanceProviderAdapter creates a new adapter for AKS instance provider

func NewAKSInstanceProviderAdapter(provider *instance.Provider) InstanceProvider {

	return &AKSInstanceProviderAdapter{

		provider: provider,
	}

}

// Create implements InstanceProvider interface

func (a *AKSInstanceProviderAdapter) Create(ctx context.Context, nodeClaim *karpenterv1.NodeClaim) (*instance.Instance, error) {

	return a.provider.Create(ctx, nodeClaim)

}

// Get implements InstanceProvider interface

func (a *AKSInstanceProviderAdapter) Get(ctx context.Context, providerID string) (*instance.Instance, error) {

	return a.provider.Get(ctx, providerID)

}

// List implements InstanceProvider interface

func (a *AKSInstanceProviderAdapter) List(ctx context.Context) ([]*instance.Instance, error) {

	return a.provider.List(ctx)

}

// Delete implements InstanceProvider interface

func (a *AKSInstanceProviderAdapter) Delete(ctx context.Context, providerID string) error {

	return a.provider.Delete(ctx, providerID)

}
