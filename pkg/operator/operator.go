/*
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

package operator

import (
	"context"

	"github.com/aws/karpenter-core/pkg/operator"
	"github.com/azure/gpu-provisioner/pkg/auth"
	azurecache "github.com/azure/gpu-provisioner/pkg/cache"
	"github.com/azure/gpu-provisioner/pkg/providers/instance"
	"github.com/azure/gpu-provisioner/pkg/providers/instancetype"
	"github.com/azure/gpu-provisioner/pkg/providers/pricing"
	"github.com/patrickmn/go-cache"
	"knative.dev/pkg/logging"
)

// Operator is injected into the AWS CloudProvider's factories
type Operator struct {
	*operator.Operator

	UnavailableOfferingsCache *azurecache.UnavailableOfferings

	PricingProvider       *pricing.Provider
	InstanceTypesProvider *instancetype.Provider
	InstanceProvider      *instance.Provider
}

func NewOperator(ctx context.Context, operator *operator.Operator) (context.Context, *Operator) {
	azConfig, err := GetAzConfig()
	if err != nil {
		logging.FromContext(ctx).Errorf("creating Azure config, %s", err)
	}

	azClient, err := instance.CreateAzClient(azConfig)
	if err != nil {
		logging.FromContext(ctx).Errorf("creating Azure client, %s", err)
	}

	unavailableOfferingsCache := azurecache.NewUnavailableOfferings()
	pricingProvider := pricing.NewProvider(
		ctx,
		pricing.NewAPI(),
		azConfig.Location,
		operator.Elected(),
	)

	instanceTypeProvider := instancetype.NewProvider(
		azConfig.Location,
		cache.New(instancetype.InstanceTypesCacheTTL, azurecache.DefaultCleanupInterval),
		azClient.SKUClient,
		pricingProvider,
		unavailableOfferingsCache,
	)
	instanceProvider := instance.NewProvider(
		azClient,
		operator.GetClient(),
		instanceTypeProvider,
		unavailableOfferingsCache,
		azConfig.Location,
		azConfig.ResourceGroup,
		azConfig.NodeResourceGroup,
		azConfig.ClusterName,
	)

	return ctx, &Operator{
		Operator:                  operator,
		UnavailableOfferingsCache: unavailableOfferingsCache,
		PricingProvider:           pricingProvider,
		InstanceTypesProvider:     instanceTypeProvider,
		InstanceProvider:          instanceProvider,
	}
}

func GetAzConfig() (*auth.Config, error) {
	cfg, err := auth.BuildAzureConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
