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

package instancetype

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"

	kcache "github.com/gpu-vmprovisioner/pkg/cache"
	"github.com/patrickmn/go-cache"
	"knative.dev/pkg/logging"

	"github.com/aws/karpenter-core/pkg/cloudprovider"

	"github.com/gpu-vmprovisioner/pkg/apis/v1alpha1"
	"github.com/gpu-vmprovisioner/pkg/providers/pricing"

	"github.com/Azure/skewer"
)

const (
	InstanceTypesCacheKey = "types"
	InstanceTypesCacheTTL = 23 * time.Hour // AWS uses 5 min here. TODO: check on why that frequent. Pricing?
)

type Provider struct {
	sync.Mutex
	region               string
	resourceSkusClient   skewer.ResourceClient
	pricingProvider      *pricing.Provider
	unavailableOfferings *kcache.UnavailableOfferings
	// Has one cache entry for all the instance types (key: InstanceTypesCacheKey)
	cache *cache.Cache
}

func NewProvider(region string, cache *cache.Cache, resourceSkusClient skewer.ResourceClient, pricingProvider *pricing.Provider, offeringsCache *kcache.UnavailableOfferings) *Provider {
	return &Provider{
		// TODO: skewer api, subnetprovider, pricing provider, unavailable offerings, ...
		region:               region,
		resourceSkusClient:   resourceSkusClient,
		pricingProvider:      pricingProvider,
		unavailableOfferings: offeringsCache,
		cache:                cache,
	}
}

// List Get all instance type options
func (p *Provider) List(
	ctx context.Context, kc *v1alpha5.KubeletConfiguration) ([]*cloudprovider.InstanceType, error) {
	p.Lock()
	defer p.Unlock()
	// Get SKUs from Azure
	skus, err := p.getInstanceTypes(ctx)
	if err != nil {
		return nil, err
	}

	// Get Viable offerings

	var result []*cloudprovider.InstanceType
	for _, sku := range skus {
		instanceType := NewInstanceType(ctx, sku, kc, p.region, p.createOfferings(ctx, sku))
		if len(instanceType.Offerings) == 0 {
			continue
		}
		result = append(result, instanceType)
	}
	return result, nil
}

func (p *Provider) LivenessProbe(req *http.Request) error {
	p.Lock()
	//nolint: staticcheck
	p.Unlock()
	return p.pricingProvider.LivenessProbe(req)
}

func (p *Provider) createOfferings(ctx context.Context, sku *skewer.SKU) []cloudprovider.Offering {

	var offerings []cloudprovider.Offering
	onDemandPrice, ok := p.pricingProvider.OnDemandPrice(*sku.Name)

	if !p.unavailableOfferings.IsUnavailable(*sku.Name, p.region, v1alpha1.PriorityRegular) {
		offerings = append(offerings, cloudprovider.Offering{Zone: "", CapacityType: v1alpha1.PriorityRegular, Price: onDemandPrice, Available: ok})
	}
	return offerings
}

// getInstanceTypes retrieves all instance types from skewer using some opinionated filters
func (p *Provider) getInstanceTypes(ctx context.Context) (map[string]*skewer.SKU, error) {
	if cached, ok := p.cache.Get(InstanceTypesCacheKey); ok {
		return cached.(map[string]*skewer.SKU), nil
	}
	instanceTypes := map[string]*skewer.SKU{}

	// TODO: filter!
	cache, err := skewer.NewCache(ctx, skewer.WithLocation(p.region), skewer.WithResourceClient(p.resourceSkusClient))
	if err != nil {
		return nil, fmt.Errorf("fetching SKUs using skewer, %w", err)
	}

	skus := cache.List(ctx, skewer.ResourceTypeFilter(skewer.VirtualMachines))
	for i := range skus {
		if p.filter(&skus[i]) {
			instanceTypes[skus[i].GetName()] = &skus[i]
		}
	}

	logging.FromContext(ctx).Debugf("Discovered %d SKUs for region %s", len(instanceTypes), p.region)
	p.cache.SetDefault(InstanceTypesCacheKey, instanceTypes)
	return instanceTypes, nil
}

// filter the instance types to include useful ones for Kubernetes
func (p *Provider) filter(_ *skewer.SKU) bool {
	// TODO: filter. AWS provider filters out FPGA and older GPU instances (see comment there)
	return true
}
