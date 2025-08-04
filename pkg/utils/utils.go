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

package utils

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ParseAgentPoolNameFromID parses the id stored on the instance ID
// Supports both AKS and Arc platform formats
func ParseAgentPoolNameFromID(id string) (string, error) {
	// Try AKS format first
	if strings.HasPrefix(id, "azure://") {
		return parseAKSAgentPoolNameFromID(id)
	}

	// Try Arc format
	if strings.HasPrefix(id, "moc://") {
		return parseArcAgentPoolNameFromID(id)
	}

	return "", fmt.Errorf("unsupported ID format: %s", id)
}

// parseAKSAgentPoolNameFromID parses AKS format IDs
func parseAKSAgentPoolNameFromID(id string) (string, error) {
	///subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachineScaleSets/<VMSSName>/virtualMachines/0
	r := regexp.MustCompile(`azure:///subscriptions/.*/resourceGroups/.*/providers/Microsoft.Compute/virtualMachineScaleSets/(?P<VMSSName>.*)/virtualMachines/.*`)
	matches := r.FindStringSubmatch(id)
	if matches == nil {
		return "", fmt.Errorf("id does not match the regex for AKS ParseAgentPoolNameFromID %s", id)
	}

	for i, name := range r.SubexpNames() {
		if name == "VMSSName" {
			nodeName := matches[i]
			agentPoolName := strings.Split(nodeName, "-") // agentpool name is the second substring
			if len(agentPoolName) == 0 {
				return "", fmt.Errorf("cannot parse agentpool name for AKS ParseAgentPoolNameFromID %s", id)
			}
			return agentPoolName[1], nil
		}
	}
	return "", fmt.Errorf("error while parsing AKS id %s", id)
}

// parseArcAgentPoolNameFromID parses Arc format IDs
// /e.g. moc://kaito-c93a5c39-gpuvmv1-md-dq8c8-ntvb7
// Pattern: moc://<cluster>-<hash>-<agentpool>-md-<hash>-<hash>
func parseArcAgentPoolNameFromID(id string) (string, error) {
	r := regexp.MustCompile(`moc://[^-]+-[^-]+-(?P<AgentPoolName>[^-]+)-md-[^-]+-[^-]+`)
	matches := r.FindStringSubmatch(id)
	if matches == nil {
		return "", fmt.Errorf("id does not match the regex for Arc ParseAgentPoolNameFromID %s", id)
	}

	for i, name := range r.SubexpNames() {
		if name == "AgentPoolName" {
			return matches[i], nil
		}
	}
	return "", fmt.Errorf("error while parsing Arc id %s", id)
}

// WithDefaultBool returns the boolean value of the supplied environment variable or, if not present,
// the supplied default value.
func WithDefaultBool(key string, def bool) bool {
	val, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	parsedVal, err := strconv.ParseBool(val)
	if err != nil {
		return def
	}
	return parsedVal
}
