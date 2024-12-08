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
	"maps"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/azure/gpu-provisioner/pkg/auth"
	"github.com/azure/gpu-provisioner/pkg/utils"
	armopts "github.com/azure/gpu-provisioner/pkg/utils/opts"
	"github.com/google/uuid"
	"k8s.io/klog/v2"
)

const (
	RPReferer = "rp.e2e.ig.e2e-aks.azure.com"
)

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
	cred, err := auth.NewCredential(cfg, azClientConfig.Authorizer)
	if err != nil {
		return nil, err
	}

	isE2E := utils.WithDefaultBool("E2E_TEST_MODE", false)
	//	If not E2E, we use the default options
	opts := armopts.DefaultArmOpts()
	if isE2E {
		opts = setArmClientOptions()
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

func setArmClientOptions() *arm.ClientOptions {
	opt := new(arm.ClientOptions)

	opt.PerCallPolicies = append(opt.PerCallPolicies,
		PolicySetHeaders{
			"Referer": []string{RPReferer},
		},
		PolicySetHeaders{
			"x-ms-correlation-request-id": []string{uuid.New().String()},
		},
	)
	opt.Cloud.Services = maps.Clone(opt.Cloud.Services) // we need this because map is a reference type
	opt.Cloud.Services[cloud.ResourceManager] = cloud.ServiceConfiguration{
		Audience: cloud.AzurePublic.Services[cloud.ResourceManager].Audience,
		Endpoint: "https://" + RPReferer,
	}
	return opt
}

// PolicySetHeaders sets http header
type PolicySetHeaders http.Header

func (p PolicySetHeaders) Do(req *policy.Request) (*http.Response, error) {
	header := req.Raw().Header
	for k, v := range p {
		header[k] = v
	}
	return req.Next()
}
