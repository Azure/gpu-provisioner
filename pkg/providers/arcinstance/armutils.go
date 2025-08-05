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

package arcinstance

import (
	"context"

	sdkerrors "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/hybridcontainerservice/armhybridcontainerservice"
	"k8s.io/klog/v2"
)

func createAgentPool(ctx context.Context, client AgentPoolsAPI, connectedClusterResourceURI, apName string, ap armhybridcontainerservice.AgentPool) (*armhybridcontainerservice.AgentPool, error) {
	klog.InfoS("createAgentPool", "agentpool", apName)

	poller, err := client.BeginCreateOrUpdate(ctx, connectedClusterResourceURI, apName, ap, nil)
	if err != nil {
		return nil, err
	}
	res, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &res.AgentPool, nil
}

func deleteAgentPool(ctx context.Context, client AgentPoolsAPI, connectedClusterResourceURI, apName string) error {
	klog.InfoS("deleteAgentPool", "agentpool", apName)
	poller, err := client.BeginDelete(ctx, connectedClusterResourceURI, apName, nil)
	if err != nil {
		azErr := sdkerrors.IsResponseError(err)
		if azErr != nil && azErr.ErrorCode == "NotFound" {
			return nil
		}
		return err
	}
	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		azErr := sdkerrors.IsResponseError(err)
		if azErr != nil && azErr.ErrorCode == "NotFound" {
			return nil
		}
	}
	return err
}

func getAgentPool(ctx context.Context, client AgentPoolsAPI, connectedClusterResourceURI, apName string) (*armhybridcontainerservice.AgentPool, error) {
	resp, err := client.Get(ctx, connectedClusterResourceURI, apName, nil)
	if err != nil {
		return nil, err
	}

	return &resp.AgentPool, nil
}

func listAgentPools(ctx context.Context, client AgentPoolsAPI, connectedClusterResourceURI string) ([]*armhybridcontainerservice.AgentPool, error) {
	var apList []*armhybridcontainerservice.AgentPool
	pager := client.NewListByProvisionedClusterPager(connectedClusterResourceURI, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		apList = append(apList, page.Value...)
	}
	return apList, nil
}
