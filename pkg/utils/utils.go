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

package utils

import (
	"fmt"
	"regexp"

	"github.com/aws/karpenter-core/pkg/cloudprovider"
)

// ParseAgentPoolNameFromID parses the id stored on the instance ID
func ParseAgentPoolNameFromID(id string) (*string, error) {
	//agentPool ID format: azure:///subscriptions/{subscriptionId}/resourcegroups/{resourceGroupName}/providers/Microsoft.ContainerService/managedClusters/{resourceName}/agentPools/{agentPoolName}
	r := regexp.MustCompile(`azure:///subscriptions/.*/resourceGroups/.*/providers/Microsoft.ContainerService/managedClusters/.*/agentPools/(?P<AgentPoolName>.*)`)
	matches := r.FindStringSubmatch(id)
	if matches == nil {
		return nil, fmt.Errorf("id does not match the regxp for ParseAgentPoolNameFromID %s", id)
	}

	for i, name := range r.SubexpNames() {
		if name == "AgentPoolName" {
			return &matches[i], nil
		}
	}
	return nil, fmt.Errorf("error while parsing id %s", id)
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
