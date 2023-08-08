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

package fake

import (
	"context"

	//nolint SA1019 - deprecated package
	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-12-01/compute"
	"github.com/Azure/skewer"
)

// TODO: consider using fakes from skewer itself

// assert that the fake implements the interface
var _ skewer.ResourceClient = &ResourceSKUsAPI{}

type ResourceSKUsAPI struct {
	// skewer.ResourceClient
}

// Reset must be called between tests otherwise tests will pollute each other.
func (s *ResourceSKUsAPI) Reset() {
	//c.ResourceSKUsBehavior.Reset()
}

func (s *ResourceSKUsAPI) ListComplete(_ context.Context, _, _ string) (compute.ResourceSkusResultIterator, error) {
	return compute.NewResourceSkusResultIterator(
		compute.NewResourceSkusResultPage(
			// cur
			compute.ResourceSkusResult{
				Value: &ResourceSkus,
			},
			// fn
			func(ctx context.Context, result compute.ResourceSkusResult) (compute.ResourceSkusResult, error) {
				return compute.ResourceSkusResult{
					Value:    nil, // end of iterator
					NextLink: nil,
				}, nil
			},
		),
	), nil
}
