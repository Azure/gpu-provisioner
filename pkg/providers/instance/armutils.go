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

package instance

import (
	"context"
	"strings"

	sdkerrors "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"k8s.io/klog/v2"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

func createAgentPool(ctx context.Context, client AgentPoolsAPI, rg, apName, clusterName string, ap armcontainerservice.AgentPool) (*armcontainerservice.AgentPool, error) {
	klog.InfoS("createAgentPool", "agentpool", apName)

	poller, err := client.BeginCreateOrUpdate(ctx, rg, clusterName, apName, ap, nil)
	if err != nil {
		return nil, err
	}
	res, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &res.AgentPool, nil
}

func deleteAgentPool(ctx context.Context, client AgentPoolsAPI, rg, clusterName, apName string) error {
	klog.InfoS("deleteAgentPool", "agentpool", apName)
	ap, err := getAgentPool(ctx, client, rg, clusterName, apName)
	if err != nil {
		return err
	}
	klog.InfoS("deleting agentpool", "agentpool", apName, "provisioningState", *ap.Properties.ProvisioningState, "powerState", func() string {
		if ap.Properties.PowerState != nil && ap.Properties.PowerState.Code != nil {
			return string(*ap.Properties.PowerState.Code)
		}
		return "unknown"
	}())

	if *ap.Properties.ProvisioningState == "Deleting" {
		klog.InfoS("agentpool is already deleting, skip delete", "agentpool", apName)
		return nil
	}

	poller, err := client.BeginDelete(ctx, rg, clusterName, apName, nil)
	if err != nil {
		azErr := sdkerrors.IsResponseError(err)
		if azErr != nil && azErr.ErrorCode == "NotFound" {
			return cloudprovider.NewNodeClaimNotFoundError(err)
		}
		return err
	}
	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		azErr := sdkerrors.IsResponseError(err)
		if azErr != nil && azErr.ErrorCode == "NotFound" {
			return cloudprovider.NewNodeClaimNotFoundError(err)
		}
	}
	return err
}

func getAgentPool(ctx context.Context, client AgentPoolsAPI, rg, clusterName, apName string) (*armcontainerservice.AgentPool, error) {
	resp, err := client.Get(ctx, rg, clusterName, apName, nil)
	if err != nil {
		if strings.Contains(err.Error(), "Agent Pool not found") {
			return nil, cloudprovider.NewNodeClaimNotFoundError(err)
		}
		return nil, err
	}

	return &resp.AgentPool, nil
}

func listAgentPools(ctx context.Context, client AgentPoolsAPI, rg, clusterName string) ([]*armcontainerservice.AgentPool, error) {
	var apList []*armcontainerservice.AgentPool
	pager := client.NewListPager(rg, clusterName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		apList = append(apList, page.Value...)
	}
	return apList, nil
}
