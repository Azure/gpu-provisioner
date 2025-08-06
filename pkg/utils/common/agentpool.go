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

package common

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (

	// Common agent pool constants

	LabelMachineType = "kaito.sh/machine-type"

	NodeClaimCreationLabel = "kaito.sh/creation-timestamp"

	// use self-defined layout in order to satisfy node label syntax

	CreationTimestampLayout = "2006-01-02T15-04-05Z"
)

var (

	// KaitoNodeLabels are the labels that identify Kaito-owned resources

	KaitoNodeLabels = []string{"kaito.sh/workspace", "kaito.sh/ragengine"}
)

// CreateAgentPoolLabels creates common labels for agent pools

func CreateAgentPoolLabels(nodeClaim *karpenterv1.NodeClaim, vmSize string) map[string]*string {

	// Base labels

	labels := map[string]*string{karpenterv1.NodePoolLabelKey: to.Ptr("kaito")}

	// Add labels from nodeClaim

	for k, v := range nodeClaim.Labels {

		labels[k] = to.Ptr(v)

	}

	// Add machine type based on VM size

	if strings.Contains(vmSize, "Standard_N") {

		labels = lo.Assign(labels, map[string]*string{LabelMachineType: to.Ptr("gpu")})

	} else {

		labels = lo.Assign(labels, map[string]*string{LabelMachineType: to.Ptr("cpu")})

	}

	// Add creation timestamp

	labels[NodeClaimCreationLabel] = to.Ptr(nodeClaim.CreationTimestamp.UTC().Format(CreationTimestampLayout))

	return labels

}

// CreateAgentPoolTaints converts Kubernetes taints to Azure format

func CreateAgentPoolTaints(taints []v1.Taint) []*string {

	taintsStr := []*string{}

	for _, t := range taints {

		taintsStr = append(taintsStr, to.Ptr(fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect)))

	}

	return taintsStr

}

// AgentPoolIsOwnedByKaito checks if an agent pool is owned by Kaito

func AgentPoolIsOwnedByKaito(nodeLabels map[string]*string) bool {

	if nodeLabels == nil {

		return false

	}

	// when agentpool.NodeLabels includes labels from kaito, return true, if not, return false

	for i := range KaitoNodeLabels {

		if _, ok := nodeLabels[KaitoNodeLabels[i]]; ok {

			return true

		}

	}

	return false

}

// AgentPoolIsCreatedFromNodeClaim checks if an agent pool was created from a NodeClaim

func AgentPoolIsCreatedFromNodeClaim(nodeLabels map[string]*string) bool {

	if nodeLabels == nil {

		return false

	}

	// when agentpool.NodeLabels includes nodepool label, return true, if not, return false

	if _, ok := nodeLabels[karpenterv1.NodePoolLabelKey]; ok {

		return true

	}

	return false

}
