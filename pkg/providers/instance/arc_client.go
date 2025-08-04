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
	"fmt"

	// TODO: Add when armhybridcontainerservice package is available
	// "github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	// "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/azure/gpu-provisioner/pkg/auth"
	// "github.com/azure/gpu-provisioner/pkg/utils"
	// armopts "github.com/azure/gpu-provisioner/pkg/utils/opts"
	// "k8s.io/klog/v2"
)

// TODO: Replace with actual armhybridcontainerservice types when package is available
type ArcAgentPool struct {
	Name       *string
	ID         *string
	Properties *ArcAgentPoolProperties
}

type ArcAgentPoolProperties struct {
	ProvisioningState interface{}
	VMSize            *string
	Count             *int32
	NodeLabels        map[string]*string
	Tags              map[string]*string
	VnetSubnetID      *string
}

type ArcAgentPoolsAPI interface {
	BeginCreateOrUpdate(ctx context.Context, connectedClusterResourceURI string, agentPoolName string, agentPool ArcAgentPool, options interface{}) (*runtime.Poller[ArcAgentPoolCreateResponse], error)
	Get(ctx context.Context, connectedClusterResourceURI string, agentPoolName string, options interface{}) (ArcAgentPoolGetResponse, error)
	BeginDelete(ctx context.Context, connectedClusterResourceURI string, agentPoolName string, options interface{}) (*runtime.Poller[ArcAgentPoolDeleteResponse], error)
	NewListByProvisionedClusterPager(connectedClusterResourceURI string, options interface{}) *runtime.Pager[ArcAgentPoolListResponse]
}

// TODO: Replace with actual response types when package is available
type ArcAgentPoolCreateResponse struct {
	AgentPool ArcAgentPool
}

type ArcAgentPoolGetResponse struct {
	AgentPool ArcAgentPool
}

type ArcAgentPoolDeleteResponse struct{}

type ArcAgentPoolListResponse struct {
	Value []*ArcAgentPool
}

type ArcAZClient struct {
	agentPoolsClient ArcAgentPoolsAPI
}

// Implement AgentPoolClient interface directly in ArcAZClient
func (c *ArcAZClient) CreateOrUpdate(ctx context.Context, params AgentPoolParams) (*AgentPoolInfo, error) {
	agentPool, ok := params.AgentPoolSpec.(ArcAgentPool)
	if !ok {
		return nil, fmt.Errorf("invalid agent pool spec type for Arc: expected ArcAgentPool")
	}

	connectedClusterURI := c.buildConnectedClusterURI(params)

	poller, err := c.agentPoolsClient.BeginCreateOrUpdate(
		ctx,
		connectedClusterURI,
		params.AgentPoolName,
		agentPool,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to begin create or update agent pool: %w", err)
	}

	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to poll until done: %w", err)
	}

	return c.convertToAgentPoolInfo(&resp.AgentPool), nil
}

func (c *ArcAZClient) Get(ctx context.Context, params AgentPoolParams) (*AgentPoolInfo, error) {
	connectedClusterURI := c.buildConnectedClusterURI(params)

	resp, err := c.agentPoolsClient.Get(
		ctx,
		connectedClusterURI,
		params.AgentPoolName,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent pool: %w", err)
	}

	return c.convertToAgentPoolInfo(&resp.AgentPool), nil
}

func (c *ArcAZClient) Delete(ctx context.Context, params AgentPoolParams) error {
	connectedClusterURI := c.buildConnectedClusterURI(params)

	poller, err := c.agentPoolsClient.BeginDelete(
		ctx,
		connectedClusterURI,
		params.AgentPoolName,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to begin delete agent pool: %w", err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to poll until done: %w", err)
	}

	return nil
}

func (c *ArcAZClient) List(ctx context.Context, params AgentPoolParams) ([]*AgentPoolInfo, error) {
	connectedClusterURI := c.buildConnectedClusterURI(params)

	pager := c.agentPoolsClient.NewListByProvisionedClusterPager(
		connectedClusterURI,
		nil,
	)

	var agentPools []*AgentPoolInfo
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get next page: %w", err)
		}

		for _, ap := range page.Value {
			agentPools = append(agentPools, c.convertToAgentPoolInfo(ap))
		}
	}

	return agentPools, nil
}

func (c *ArcAZClient) buildConnectedClusterURI(params AgentPoolParams) string {
	// For Azure Kubernetes connected clusters, we need to construct the connected cluster resource URI
	// Format: /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Kubernetes/connectedClusters/{clusterName}
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Kubernetes/connectedClusters/%s",
		params.SubscriptionID,
		params.ResourceGroup,
		params.ClusterName,
	)
}

func (c *ArcAZClient) convertToAgentPoolInfo(ap *ArcAgentPool) *AgentPoolInfo {
	if ap == nil || ap.Properties == nil {
		return nil
	}

	return &AgentPoolInfo{
		Name:              ap.Name,
		ID:                ap.ID,
		ProvisioningState: ap.Properties.ProvisioningState,
		VMSize:            ap.Properties.VMSize,
		Count:             ap.Properties.Count,
		NodeLabels:        ap.Properties.NodeLabels,
		Tags:              ap.Properties.Tags,
		VnetSubnetID:      ap.Properties.VnetSubnetID,
		// Note: Arc may not have NodeImageVersion, adjust as needed
	}
}

func NewArcAZClientFromAPI(
	agentPoolsClient ArcAgentPoolsAPI,
) *ArcAZClient {
	return &ArcAZClient{
		agentPoolsClient: agentPoolsClient,
	}
}

func CreateArcAzClient(cfg *auth.Config) (*ArcAZClient, error) {
	// Defaulting env to Azure Public Cloud.
	env := azure.PublicCloud
	var err error

	azClient, err := NewArcAZClient(cfg, &env)
	if err != nil {
		return nil, err
	}

	return azClient, nil
}

func NewArcAZClient(cfg *auth.Config, env *azure.Environment) (*ArcAZClient, error) {
	// TODO: Implement Arc Azure client when armhybridcontainerservice package is available
	// For now, return an error indicating this is not yet implemented
	return nil, fmt.Errorf("Arc agent pool client not yet implemented - armhybridcontainerservice package not available")

	// The actual implementation will be similar to NewAKSAZClient but using Arc packages
	/*
		var cred azcore.TokenCredential
		var err error

		if cfg.DeploymentMode == "managed" {
			cred, err = azidentity.NewDefaultAzureCredential(nil)
		} else {
			// deploymentMode value is "self-hosted" or "", then use the federated identity.
			authorizer, uerr := auth.NewAuthorizer(cfg, env)
			if uerr != nil {
				return nil, uerr
			}
			azClientConfig := cfg.GetAzureClientConfig(authorizer, env)
			azClientConfig.UserAgent = auth.GetUserAgentExtension()
			cred, err = auth.NewCredential(cfg, azClientConfig.Authorizer)
		}

		if err != nil {
			return nil, err
		}

		isE2E := utils.WithDefaultBool("E2E_TEST_MODE", false)
		//	If not E2E, we use the default options
		opts := armopts.DefaultArmOpts()
		if isE2E {
			opts = setArmClientOptions()
		}

		agentPoolClient, err := armhybridcontainerservice.NewAgentPoolClient(cred, opts)
		if err != nil {
			return nil, err
		}
		klog.V(5).Infof("Created agent pool client %v using token credential", agentPoolClient)

		return &ArcAZClient{
			agentPoolsClient: agentPoolClient,
		}, nil
	*/
}
