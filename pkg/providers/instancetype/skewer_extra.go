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

import "github.com/Azure/skewer"

const (
	// CapabilityCpuArchitectureType identifies the type of CPU architecture (x64,Arm64).
	CapabilityCPUArchitectureType = "CpuArchitectureType"

	// CapabilityPremiumIO
	CapabilityPremiumIO = "PremiumIO"
)

func IsHyperVGen1Supported(s *skewer.SKU) bool {
	return s.HasCapabilityWithSeparator(skewer.HyperVGenerations, skewer.HyperVGeneration1)
}

// GetCapability retrieves string capability with the provided name.
// It errors if the capability is not found or the value was nil
func GetCapability(s *skewer.SKU, name string) (string, error) {
	if s.Capabilities == nil {
		return "", &skewer.ErrCapabilityNotFound{} // should be {name}, but capability field is not exported
	}
	for _, capability := range *s.Capabilities {
		if capability.Name != nil && *capability.Name == name {
			if capability.Value != nil {
				return *capability.Value, nil
			}
			return "", &skewer.ErrCapabilityValueNil{}
		}
	}
	return "", &skewer.ErrCapabilityNotFound{}
}

func GetCPUArchitectureType(s *skewer.SKU) (string, error) {
	return GetCapability(s, CapabilityCPUArchitectureType)
}

func IsPremiumIO(s *skewer.SKU) bool {
	return s.HasCapability(CapabilityPremiumIO)
}
