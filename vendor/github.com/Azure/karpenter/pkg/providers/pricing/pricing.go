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

package pricing

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Azure/karpenter/pkg/providers/pricing/client"
	"github.com/aws/karpenter-core/pkg/utils/pretty"
	"github.com/samber/lo"
	"knative.dev/pkg/logging"
)

// pricingUpdatePeriod is how often we try to update our pricing information after the initial update on startup
const pricingUpdatePeriod = 12 * time.Hour

// Provider provides actual pricing data to the Azure cloud provider to allow it to make more informed decisions
// regarding which instances to launch.  This is initialized at startup with a periodically updated static price list to
// support running in locations where pricing data is unavailable.  In those cases the static pricing data provides a
// relative ordering that is still more accurate than our previous pricing model.  In the event that a pricing update
// fails, the previous pricing information is retained and used which may be the static initial pricing data if pricing
// updates never succeed.
type Provider struct {
	pricing client.PricingAPI
	region  string
	cm      *pretty.ChangeMonitor

	mu                 sync.RWMutex
	onDemandUpdateTime time.Time
	onDemandPrices     map[string]float64
}

type Err struct {
	error
	lastUpdateTime time.Time
}

// NewPricingAPI returns a pricing API
func NewAPI() client.PricingAPI {
	return client.New()
}

func NewProvider(ctx context.Context, pricing client.PricingAPI, region string, startAsync <-chan struct{}) *Provider {
	// see if we've got region specific pricing data
	staticPricing, ok := initialOnDemandPrices[region]
	if !ok {
		// and if not, fall back to the always available eastus
		staticPricing = initialOnDemandPrices["eastus"]
	}

	p := &Provider{
		region:             region,
		onDemandUpdateTime: initialPriceUpdate,
		onDemandPrices:     staticPricing,
		pricing:            pricing,
		cm:                 pretty.NewChangeMonitor(),
	}
	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).Named("pricing"))

	go func() {
		// perform an initial price update at startup
		p.updatePricing(ctx)

		startup := time.Now()
		// wait for leader election or to be signaled to exit
		select {
		case <-startAsync:
		case <-ctx.Done():
			return
		}
		// if it took many hours to be elected leader, we want to re-fetch pricing before we start our periodic
		// polling
		if time.Since(startup) > pricingUpdatePeriod {
			p.updatePricing(ctx)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(pricingUpdatePeriod):
				p.updatePricing(ctx)
			}
		}
	}()
	return p
}

// InstanceTypes returns the list of all instance types for which either a price is known.
func (p *Provider) InstanceTypes() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return lo.Keys(p.onDemandPrices)
}

// OnDemandLastUpdated returns the time that the on-demand pricing was last updated
func (p *Provider) OnDemandLastUpdated() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.onDemandUpdateTime
}

// OnDemandPrice returns the last known on-demand price for a given instance type, returning false if there is no
// known on-demand pricing for the instance type.
func (p *Provider) OnDemandPrice(instanceType string) (float64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	price, ok := p.onDemandPrices[instanceType]
	if !ok {
		return 0.0, false
	}
	return price, true
}

func (p *Provider) updatePricing(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := p.UpdateOnDemandPricing(ctx); err != nil {
			logging.FromContext(ctx).Errorf("error updating on-demand pricing for region %s, %s, using existing pricing data from %s", p.region, err, err.lastUpdateTime.Format(time.RFC3339))
		}
	}()

	wg.Wait()
}

func (p *Provider) UpdateOnDemandPricing(ctx context.Context) *Err {
	// standard on-demand instances
	var wg sync.WaitGroup
	var onDemandPrices map[string]float64
	var onDemandErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		onDemandPrices, onDemandErr = p.fetchOnDemandPricing(ctx)
	}()

	wg.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()
	err := onDemandErr
	if err != nil {
		return &Err{error: err, lastUpdateTime: p.onDemandUpdateTime}
	}

	if len(onDemandPrices) == 0 {
		return &Err{error: errors.New("no on-demand pricing found"), lastUpdateTime: p.onDemandUpdateTime}
	}

	p.onDemandPrices = lo.Assign(onDemandPrices)
	p.onDemandUpdateTime = time.Now()
	if p.cm.HasChanged("on-demand-prices", p.onDemandPrices) {
		logging.FromContext(ctx).With("instance-type-count", len(p.onDemandPrices)).Infof("updated on-demand pricing for region %s", p.region)
	}
	return nil
}

func (p *Provider) fetchOnDemandPricing(ctx context.Context) (map[string]float64, error) {
	prices := map[string]float64{}
	filters := []*client.Filter{
		{
			Field:    "priceType",
			Operator: client.Equals,
			Value:    "Consumption",
		},
		{
			Field:    "currencyCode",
			Operator: client.Equals,
			Value:    "USD",
		},
		{
			Field:    "serviceFamily",
			Operator: client.Equals,
			Value:    "Compute",
		},
		{
			Field:    "serviceName",
			Operator: client.Equals,
			Value:    "Virtual Machines",
		},
		{
			Field:    "armRegionName",
			Operator: client.Equals,
			Value:    p.region,
		}}
	if err := p.pricing.GetProductsPricePages(ctx, filters, p.onDemandPage(prices)); err != nil {
		return nil, err
	}
	return prices, nil
}

func (p *Provider) onDemandPage(prices map[string]float64) func(page *client.ProductsPricePage) {
	return func(page *client.ProductsPricePage) {
		for _, pItem := range page.Items {
			if strings.HasSuffix(pItem.ProductName, " Windows") {
				continue
			}
			if strings.HasSuffix(pItem.MeterName, " Low Priority") {
				// https://learn.microsoft.com/en-us/azure/batch/batch-spot-vms#differences-between-spot-and-low-priority-vms
				continue
			}
			if strings.HasSuffix(pItem.SkuName, " Spot") {
				continue
			}
			prices[pItem.ArmSkuName] = pItem.RetailPrice
		}
	}
}

func (p *Provider) LivenessProbe(_ *http.Request) error {
	// ensure we don't deadlock and nolint for the empty critical section
	p.mu.Lock()
	//nolint: staticcheck
	p.mu.Unlock()
	return nil
}

func (p *Provider) Reset() {
	// see if we've got region specific pricing data
	staticPricing, ok := initialOnDemandPrices[p.region]
	if !ok {
		// and if not, fall back to the always available eastus
		staticPricing = initialOnDemandPrices["eastus"]
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.onDemandPrices = staticPricing
	p.onDemandUpdateTime = initialPriceUpdate
}
