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
	"encoding/base64"
	"fmt"

	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/ptr"

	"github.com/gpu-vmprovisioner/pkg/apis/settings"
	"github.com/gpu-vmprovisioner/pkg/auth"
	azurecache "github.com/gpu-vmprovisioner/pkg/cache"
	"github.com/gpu-vmprovisioner/pkg/providers/imagefamily"
	"github.com/gpu-vmprovisioner/pkg/providers/instance"
	"github.com/gpu-vmprovisioner/pkg/providers/instancetype"
	"github.com/gpu-vmprovisioner/pkg/providers/launchtemplate"
	"github.com/gpu-vmprovisioner/pkg/providers/pricing"
	"github.com/aws/karpenter-core/pkg/operator"
)

// Operator is injected into the AWS CloudProvider's factories
type Operator struct {
	*operator.Operator

	UnavailableOfferingsCache *azurecache.UnavailableOfferings

	ImageProvider          *imagefamily.Provider
	ImageResolver          *imagefamily.Resolver
	LaunchTemplateProvider *launchtemplate.Provider
	PricingProvider        *pricing.Provider
	InstanceTypesProvider  *instancetype.Provider
	InstanceProvider       *instance.Provider
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

	imageProvider := imagefamily.NewProvider(operator.KubernetesInterface, cache.New(azurecache.KubernetesVersionTTL, azurecache.DefaultCleanupInterval))
	imageResolver := imagefamily.New(operator.GetClient(), imageProvider)
	launchTemplateProvider := launchtemplate.NewProvider(
		ctx,
		imageResolver,
		lo.Must(getCABundle(operator.GetConfig())),
		settings.FromContext(ctx).ClusterEndpoint,
		azConfig.TenantID,
		azConfig.SubscriptionID,
		azConfig.UserAssignedIdentityID,
		azConfig.NodeResourceGroup,
		azConfig.Location,
	)
	instanceTypeProvider := instancetype.NewProvider(
		azConfig.Location,
		cache.New(instancetype.InstanceTypesCacheTTL, azurecache.DefaultCleanupInterval),
		azClient.SKUClient,
		pricingProvider,
		unavailableOfferingsCache,
	)
	instanceProvider := instance.NewProvider(
		ctx,
		azClient,
		instanceTypeProvider,
		launchTemplateProvider,
		unavailableOfferingsCache,
		azConfig.Location,
		azConfig.NodeResourceGroup,
		azConfig.SubnetID,
	)

	return ctx, &Operator{
		Operator:                  operator,
		UnavailableOfferingsCache: unavailableOfferingsCache,
		ImageProvider:             imageProvider,
		ImageResolver:             imageResolver,
		LaunchTemplateProvider:    launchTemplateProvider,
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

func getCABundle(restConfig *rest.Config) (*string, error) {
	// Discover CA Bundle from the REST client. We could alternatively
	// have used the simpler client-go InClusterConfig() method.
	// However, that only works when Karpenter is running as a Pod
	// within the same cluster it's managing.
	transportConfig, err := restConfig.TransportConfig()
	if err != nil {
		return nil, fmt.Errorf("discovering caBundle, loading transport config, %w", err)
	}
	_, err = transport.TLSConfigFor(transportConfig) // fills in CAData!
	if err != nil {
		return nil, fmt.Errorf("discovering caBundle, loading TLS config, %w", err)
	}
	return ptr.String(base64.StdEncoding.EncodeToString(transportConfig.TLS.CAData)), nil
}
