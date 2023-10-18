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

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	// nolint SA1019 - deprecated package
	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2022-08-01/compute"
	"github.com/Azure/skewer"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/azure/gpu-provisioner/pkg/auth"
	armopts "github.com/azure/gpu-provisioner/pkg/utils/opts"
	klog "k8s.io/klog/v2"
)

type AgentPoolsAPI interface {
	BeginCreateOrUpdate(ctx context.Context, resourceGroupName string, resourceName string, agentPoolName string, parameters armcontainerservice.AgentPool, options *armcontainerservice.AgentPoolsClientBeginCreateOrUpdateOptions) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error)
	Get(ctx context.Context, resourceGroupName string, resourceName string, agentPoolName string, options *armcontainerservice.AgentPoolsClientGetOptions) (armcontainerservice.AgentPoolsClientGetResponse, error)
	BeginDelete(ctx context.Context, resourceGroupName string, resourceName string, agentPoolName string, options *armcontainerservice.AgentPoolsClientBeginDeleteOptions) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error)
	NewListPager(resourceGroupName string, resourceName string, options *armcontainerservice.AgentPoolsClientListOptions) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse]
}

type AZClient struct {
	agentPoolsClient AgentPoolsAPI
	// SKU CLIENT is still using track 1 because skewer does not support the track 2 path. We need to refactor this once skewer supports track 2
	SKUClient skewer.ResourceClient
}

func NewAZClientFromAPI(
	agentPoolsClient AgentPoolsAPI,
	skuClient skewer.ResourceClient,
) *AZClient {
	return &AZClient{
		agentPoolsClient: agentPoolsClient,
		SKUClient:        skuClient,
	}
}

func CreateAzClient(cfg *auth.Config) (*AZClient, error) {
	// Defaulting env to Azure Public Cloud.
	env := azure.PublicCloud
	var err error

	azClient, err := NewAZClient(cfg, &env)
	if err != nil {
		return nil, err
	}

	return azClient, nil
}

func NewAZClient(cfg *auth.Config, env *azure.Environment) (*AZClient, error) {
	authorizer, err := auth.NewAuthorizer(cfg, env)
	if err != nil {
		return nil, err
	}

	azClientConfig := cfg.GetAzureClientConfig(authorizer, env)
	azClientConfig.UserAgent = auth.GetUserAgentExtension()
	cred, err := auth.NewCredential(cfg, env)
	if err != nil {
		return nil, err
	}

	opts := armopts.DefaultArmOpts()

	agentPoolClient, err := armcontainerservice.NewAgentPoolsClient(cfg.SubscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	klog.V(5).Infof("Created agent pool client %v using token credential", agentPoolClient)
	interfacesClient, err := armnetwork.NewInterfacesClient(cfg.SubscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	klog.V(5).Infof("Created network interface client %v using token credential", interfacesClient)

	// TODO: this one is not enabled for rate limiting / throttling ...
	// TODO Move this over to track 2 when skewer is migrated
	skuClient := compute.NewResourceSkusClient(cfg.SubscriptionID)
	skuClient.Authorizer = azClientConfig.Authorizer
	klog.V(5).Infof("Created sku client with authorizer: %v", skuClient)

	return &AZClient{
		agentPoolsClient: agentPoolClient,
		SKUClient:        skuClient,
	}, nil
}
