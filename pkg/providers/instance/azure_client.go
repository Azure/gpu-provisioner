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
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/azure/gpu-provisioner/pkg/auth"
	"github.com/azure/gpu-provisioner/pkg/utils"
	armopts "github.com/azure/gpu-provisioner/pkg/utils/opts"
	"github.com/google/uuid"
	"k8s.io/klog/v2"
)

type CloudEnvironmentName string

const (
	AzurePublicCloud       CloudEnvironmentName = "azurepubliccloud"
	AzureUSGovernmentCloud CloudEnvironmentName = "azureusgovernmentcloud"
	AzureChinaCloud        CloudEnvironmentName = "azurechinacloud"
)

// PolicySetHeaders sets http header
type PolicySetHeaders http.Header

func (p PolicySetHeaders) Do(req *policy.Request) (*http.Response, error) {
	header := req.Raw().Header
	for k, v := range p {
		header[k] = v
	}
	return req.Next()
}

type AgentPoolsAPI interface {
	BeginCreateOrUpdate(ctx context.Context, resourceGroupName string, resourceName string, agentPoolName string, parameters armcontainerservice.AgentPool, options *armcontainerservice.AgentPoolsClientBeginCreateOrUpdateOptions) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error)
	Get(ctx context.Context, resourceGroupName string, resourceName string, agentPoolName string, options *armcontainerservice.AgentPoolsClientGetOptions) (armcontainerservice.AgentPoolsClientGetResponse, error)
	BeginDelete(ctx context.Context, resourceGroupName string, resourceName string, agentPoolName string, options *armcontainerservice.AgentPoolsClientBeginDeleteOptions) (*runtime.Poller[armcontainerservice.AgentPoolsClientDeleteResponse], error)
	NewListPager(resourceGroupName string, resourceName string, options *armcontainerservice.AgentPoolsClientListOptions) *runtime.Pager[armcontainerservice.AgentPoolsClientListResponse]
}

type AZClient struct {
	agentPoolsClient AgentPoolsAPI
}

func NewAZClientFromAPI(
	agentPoolsClient AgentPoolsAPI,
) *AZClient {
	return &AZClient{
		agentPoolsClient: agentPoolsClient,
	}
}

func CreateAzClient(ctx context.Context, cfg *auth.Config) (*AZClient, error) {
	e2eMode := utils.WithDefaultBool("E2E_TEST_MODE", false)

	// Defaulting env to Azure Public Cloud.
	cloudConfig := getCloudConfiguration(cfg.CloudEnvironment, e2eMode)
	var err error

	azClient, err := NewAZClient(ctx, cfg, cloudConfig.Services[cloud.ResourceManager].Endpoint, e2eMode)
	if err != nil {
		return nil, err
	}

	return azClient, nil
}

func NewAZClient(ctx context.Context, cfg *auth.Config, resourceEndpoint string, e2eMode bool) (*AZClient, error) {
	var cred azcore.TokenCredential
	var err error
	opts := armopts.DefaultArmOpts()

	if cfg.DeploymentMode == "managed" {
		cred, err = azidentity.NewDefaultAzureCredential(nil)
	} else {
		// deploymentMode value is "self-hosted" or "", then use the federated identity.
		authorizer, uerr := auth.NewAuthorizer(ctx, cfg, resourceEndpoint)
		if uerr != nil {
			return nil, uerr
		}
		azClientConfig := cfg.GetAzureClientConfig(authorizer, resourceEndpoint)
		azClientConfig.UserAgent = auth.GetUserAgentExtension()
		cred, err = auth.NewCredential(cfg, azClientConfig.Authorizer)
	}

	if err != nil {
		return nil, err
	}

	if e2eMode {
		transporter, err := auth.GetE2ETLSConfig(ctx, cred, cfg)
		if err != nil {
			return nil, err
		}
		opts = setE2eArmClientOptions(cfg, transporter)
	}

	agentPoolClient, err := armcontainerservice.NewAgentPoolsClient(cfg.SubscriptionID, cred, opts)
	if err != nil {
		return nil, err
	}
	klog.V(5).Infof("Created agent pool client %v using token credential", agentPoolClient)

	return &AZClient{
		agentPoolsClient: agentPoolClient,
	}, nil
}

func setE2eArmClientOptions(cfg *auth.Config, transporter *http.Client) *arm.ClientOptions {

	cloudConfig := getCloudConfiguration(cfg.CloudEnvironment, true)

	return &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			PerCallPolicies: []policy.Policy{
				PolicySetHeaders(http.Header{
					"Referer": []string{
						"https://" + auth.E2E_RP_INGRESS_ENDPOINT_ADDRESS,
					},
					"x-ms-correlation-request-id": []string{uuid.New().String()},
					"x-ms-home-tenant-id":         []string{cfg.TenantID},
				}),
			},
			Transport: transporter,
			Logging: policy.LogOptions{
				IncludeBody: true,
			},
			Cloud: cloudConfig,
		},
	}
}

func getCloudConfiguration(cloudName string, e2eMode bool) cloud.Configuration {
	var cloudConfig cloud.Configuration
	switch strings.ToLower(cloudName) {
	case string(AzurePublicCloud):
		cloudConfig = cloud.AzurePublic
	case string(AzureUSGovernmentCloud):
		cloudConfig = cloud.AzureGovernment
	case string(AzureChinaCloud):
		cloudConfig = cloud.AzureChina
	default:
		panic("cloud config does not exist")
	}

	if cloudConfig.Services == nil {
		cloudConfig.Services = make(map[cloud.ServiceName]cloud.ServiceConfiguration)
	}
	if e2eMode {
		// Set the resource manager endpoint to the E2E test endpoint
		cloudConfig.Services[cloud.ResourceManager] = cloud.ServiceConfiguration{
			Audience: "https://management.azure.com/",
			Endpoint: "https://" + auth.E2E_RP_INGRESS_ENDPOINT_ADDRESS,
		}
	}
	return cloudConfig
}
