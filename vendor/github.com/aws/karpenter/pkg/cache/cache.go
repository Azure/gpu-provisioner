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

package cache

import "time"

const (
	// DefaultTTL restricts QPS to AWS APIs to this interval for verifying setup
	// resources. This value represents the maximum eventual consistency between
	// AWS actual state and the controller's ability to provision those
	// resources. Cache hits enable faster provisioning and reduced API load on
	// AWS APIs, which can have a serious impact on performance and scalability.
	// DO NOT CHANGE THIS VALUE WITHOUT DUE CONSIDERATION
	DefaultTTL = time.Minute
	// UnavailableOfferingsTTL is the time before offerings that were marked as unavailable
	// are removed from the cache and are available for launch again
	UnavailableOfferingsTTL = 3 * time.Minute
	// InstanceTypesAndZonesTTL is the time before we refresh instance types and zones at EC2
	InstanceTypesAndZonesTTL = 5 * time.Minute
)

const (
	// DefaultCleanupInterval triggers cache cleanup (lazy eviction) at this interval.
	DefaultCleanupInterval = 10 * time.Minute
)
