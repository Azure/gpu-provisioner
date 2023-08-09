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

package test

import (
	"context"

	azurecache "github.com/gpu-vmprovisioner/pkg/cache"
	"github.com/gpu-vmprovisioner/pkg/fake"
	"github.com/gpu-vmprovisioner/pkg/providers/imagefamily"
	"github.com/gpu-vmprovisioner/pkg/providers/instance"
	"github.com/gpu-vmprovisioner/pkg/providers/instancetype"
	"github.com/gpu-vmprovisioner/pkg/providers/launchtemplate"
	"github.com/gpu-vmprovisioner/pkg/providers/pricing"
	"github.com/patrickmn/go-cache"
	"knative.dev/pkg/ptr"

	coretest "github.com/aws/karpenter-core/pkg/test"
)

type Environment struct {
	// API
	VirtualMachinesAPI          *fake.VirtualMachinesAPI
	VirtualMachineExtensionsAPI *fake.VirtualMachineExtensionsAPI
	NetworkInterfacesAPI        *fake.NetworkInterfacesAPI
	ResourceSKUsAPI             *fake.ResourceSKUsAPI
	PricingAPI                  *fake.PricingAPI

	// Cache
	KubernetesVersionCache    *cache.Cache
	InstanceTypeCache         *cache.Cache
	UnavailableOfferingsCache *azurecache.UnavailableOfferings

	// Providers
	InstanceTypesProvider  *instancetype.Provider
	InstanceProvider       *instance.Provider
	PricingProvider        *pricing.Provider
	ImageProvider          *imagefamily.Provider
	ImageResolver          *imagefamily.Resolver
	LaunchTemplateProvider *launchtemplate.Provider
}

// using "" for region

func NewEnvironment(ctx context.Context, env *coretest.Environment) *Environment {
	testSettings := Settings()

	// API
	virtualMachinesAPI := &fake.VirtualMachinesAPI{}
	virtualMachinesExtensionsAPI := &fake.VirtualMachineExtensionsAPI{}
	networkInterfacesAPI := &fake.NetworkInterfacesAPI{}
	pricingAPI := &fake.PricingAPI{}
	resourceSKUsAPI := &fake.ResourceSKUsAPI{}

	// Cache
	kubernetesVersionCache := cache.New(azurecache.KubernetesVersionTTL, azurecache.DefaultCleanupInterval)
	instanceTypeCache := cache.New(instancetype.InstanceTypesCacheTTL, azurecache.DefaultCleanupInterval)
	unavailableOfferingsCache := azurecache.NewUnavailableOfferings()

	// Providers
	pricingProvider := pricing.NewProvider(ctx, pricingAPI, "", make(chan struct{}))
	imageFamilyProvider := imagefamily.NewProvider(env.KubernetesInterface, kubernetesVersionCache)
	imageFamilyResolver := imagefamily.New(env.Client, imageFamilyProvider)
	instanceTypesProvider := instancetype.NewProvider("", instanceTypeCache, resourceSKUsAPI, pricingProvider, unavailableOfferingsCache)
	launchTemplateProvider := launchtemplate.NewProvider(
		ctx,
		imageFamilyResolver,
		ptr.String("ca-bundle"),
		testSettings.ClusterEndpoint,
		"test-tenant",
		"test-subscription",
		"test-userAssignedIdentity",
		"test-resourceGroup",
		"test-location",
	)
	azClient := instance.NewAZClientFromAPI(
		virtualMachinesAPI,
		virtualMachinesExtensionsAPI,
		networkInterfacesAPI,
		resourceSKUsAPI,
	)
	instanceProvider := instance.NewProvider(
		ctx,
		azClient,
		instanceTypesProvider,
		launchTemplateProvider,
		unavailableOfferingsCache,
		"testregion", // region
		"",           // resourceGroup
		"",           // subnet
	)

	return &Environment{
		VirtualMachinesAPI:          virtualMachinesAPI,
		VirtualMachineExtensionsAPI: virtualMachinesExtensionsAPI,
		NetworkInterfacesAPI:        networkInterfacesAPI,
		ResourceSKUsAPI:             resourceSKUsAPI,
		PricingAPI:                  pricingAPI,

		KubernetesVersionCache:    kubernetesVersionCache,
		InstanceTypeCache:         instanceTypeCache,
		UnavailableOfferingsCache: unavailableOfferingsCache,
		InstanceTypesProvider:     instanceTypesProvider,
		InstanceProvider:          instanceProvider,
		PricingProvider:           pricingProvider,
		ImageProvider:             imageFamilyProvider,
		ImageResolver:             imageFamilyResolver,
		LaunchTemplateProvider:    launchTemplateProvider,
	}
}

func (env *Environment) Reset() {
	env.VirtualMachinesAPI.Reset()
	env.VirtualMachineExtensionsAPI.Reset()
	env.NetworkInterfacesAPI.Reset()
	env.ResourceSKUsAPI.Reset()
	env.PricingAPI.Reset()
	env.PricingProvider.Reset()

	env.KubernetesVersionCache.Flush()
	env.InstanceTypeCache.Flush()
	env.UnavailableOfferingsCache.Flush()
}
