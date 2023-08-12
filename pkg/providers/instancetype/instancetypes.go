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

	"github.com/samber/lo"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"

	kcache "github.com/gpu-vmprovisioner/pkg/cache"
	"github.com/patrickmn/go-cache"
	"k8s.io/apimachinery/pkg/util/sets"
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

// Get all instance type options
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
	/// AWS here gets general zone availability info for all instanceTypes (filtered by subnet) and uses it later
	/// Azure is going to have zones availability directly from SKU info
	var result []*cloudprovider.InstanceType
	for _, sku := range skus {
		instanceTypeZones := instanceTypeZones(sku, p.region)
		instanceType := NewInstanceType(ctx, sku, kc, p.region, p.createOfferings(sku, instanceTypeZones))
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

// instanceTypeZones generates the set of all supported zones for a given SKU
// The strings have to match Zone labels that will be placed on Node
func instanceTypeZones(sku *skewer.SKU, region string) sets.String {
	// skewer returns numerical zones, like "1" (as keys in the map);
	// prefix each zone with "<region>-", to have them match the labels placed on Node (e.g. "westus2-1")
	return sets.NewString(lo.Map(lo.Keys(sku.AvailabilityZones(region)), func(zone string, _ int) string {
		return fmt.Sprintf("%s-%s", region, zone)
	})...)
}

func (p *Provider) createOfferings(sku *skewer.SKU, zones sets.String) []cloudprovider.Offering {
	// TODO: AWS provider filters out offerings with recently unavailable capacity
	// TODO: currently assumes each SKU can be either spot or regular (on-demand) (likely wrong? In price sheets I see SKUs with no spot prices ...)
	spotRatio := .20 // just a guess at savings
	offerings := []cloudprovider.Offering{}
	for zone := range zones {
		onDemandPrice, ok := p.pricingProvider.OnDemandPrice(*sku.Name)
		spotPrice := onDemandPrice * spotRatio

		if !p.unavailableOfferings.IsUnavailable(*sku.Name, p.region, v1alpha1.PrioritySpot) {
			offerings = append(offerings, cloudprovider.Offering{Zone: zone, CapacityType: v1alpha1.PrioritySpot, Price: spotPrice, Available: ok})
		}
		if !p.unavailableOfferings.IsUnavailable(*sku.Name, p.region, v1alpha1.PriorityRegular) {
			offerings = append(offerings, cloudprovider.Offering{Zone: zone, CapacityType: v1alpha1.PriorityRegular, Price: onDemandPrice, Available: ok})
		}
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

	logging.FromContext(ctx).Debugf("Discovered %d SKUs", len(instanceTypes))
	p.cache.SetDefault(InstanceTypesCacheKey, instanceTypes)
	return instanceTypes, nil
}

// filter the instance types to include useful ones for Kubernetes
func (p *Provider) filter(_ *skewer.SKU) bool {
	// TODO: filter. AWS provider filters out FPGA and older GPU instances (see comment there)
	return true
}
