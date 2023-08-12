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

package instance

import (
	"context"

	sdkerrors "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
)

func createAgentPool(ctx context.Context, client AgentPoolsAPI, rg, apName, clusterName string, ap armcontainerservice.AgentPool) (*armcontainerservice.AgentPool, error) {
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

func deleteAgentPool(ctx context.Context, client AgentPoolsAPI, rg, apName, clusterName string) error {
	poller, err := client.BeginDelete(ctx, rg, clusterName, apName, nil)
	if err != nil {
		return err
	}
	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		if sdkerrors.IsNotFoundErr(err) {
			return nil
		}
	}
	return err
}

func createNic(ctx context.Context, client NetworkInterfacesAPI, rg, nicName string, nic armnetwork.Interface) (*armnetwork.Interface, error) {
	poller, err := client.BeginCreateOrUpdate(ctx, rg, nicName, nic, nil)
	if err != nil {
		return nil, err
	}
	res, err := poller.PollUntilDone(ctx, nil)

	if err != nil {
		return nil, err
	}
	return &res.Interface, nil
}
