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

	"github.com/aws/karpenter-core/pkg/cloudprovider"
)

// ParseAgentPoolNameFromID parses the id stored on the instance ID
func ParseAgentPoolNameFromID(id string) (string, error) {
	///subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachineScaleSets/<VMSSName>/virtualMachines/0
	r := regexp.MustCompile(`azure:///subscriptions/.*/resourceGroups/.*/providers/Microsoft.Compute/virtualMachineScaleSets/(?P<VMSSName>.*)/virtualMachines/.*`)
	matches := r.FindStringSubmatch(id)
	if matches == nil {
		return "", fmt.Errorf("id does not match the regxp for ParseAgentPoolNameFromID %s", id)
	}

	for i, name := range r.SubexpNames() {
		if name == "VMSSName" {
			nodeName := matches[i]
			agentPoolName := strings.Split(nodeName, "-") // agentpool name is the second substring
			if len(agentPoolName) == 0 {
				return "", fmt.Errorf("cannot parse agentpool name for ParseAgentPoolNameFromID %s", id)
			}
			return agentPoolName[1], nil
		}
	}
	return "", fmt.Errorf("error while parsing id %s", id)
}

// ParseSubIDFromID parses the id stored on the instance ID
func ParseSubIDFromID(id string) (*string, error) {
	r := regexp.MustCompile(`/subscriptions/(?P<SubscriptionID>.*)/resourcegroups/.*/providers/.*`)
	matches := r.FindStringSubmatch(id)
	if matches == nil {
		return nil, fmt.Errorf("id does not match the regxp for ParseSubIDFromID %s", id)
	}

	for i, name := range r.SubexpNames() {
		if name == "SubscriptionID" {
			return &matches[i], nil
		}
	}
	return nil, fmt.Errorf("error while parsing id %s", id)
}

func GetAllSingleValuedRequirementLabels(instanceType *cloudprovider.InstanceType) map[string]string {
	labels := map[string]string{}
	if instanceType == nil {
		return labels
	}
	for key, req := range instanceType.Requirements {
		if req.Len() == 1 {
			labels[key] = req.Values()[0]
		}
	}
	return labels
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
