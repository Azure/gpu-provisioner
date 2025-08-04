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
	"maps"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/azure/gpu-provisioner/pkg/auth"
	"github.com/azure/gpu-provisioner/pkg/utils"
	armopts "github.com/azure/gpu-provisioner/pkg/utils/opts"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
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

// Implement AgentPoolClient interface directly in AZClient
func (c *AZClient) CreateOrUpdate(ctx context.Context, params AgentPoolParams) (*AgentPoolInfo, error) {
	// Convert NodeClaim to AKS AgentPool internally
	agentPool, err := c.nodeClaimToAgentPool(params.VMSize, params.NodeClaim)
	if err != nil {
		return nil, fmt.Errorf("failed to convert NodeClaim to AgentPool: %w", err)
	}

	poller, err := c.agentPoolsClient.BeginCreateOrUpdate(
		ctx,
		params.ResourceGroup,
		params.ClusterName,
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

func (c *AZClient) Get(ctx context.Context, params AgentPoolParams) (*AgentPoolInfo, error) {
	resp, err := c.agentPoolsClient.Get(
		ctx,
		params.ResourceGroup,
		params.ClusterName,
		params.AgentPoolName,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent pool: %w", err)
	}

	return c.convertToAgentPoolInfo(&resp.AgentPool), nil
}

func (c *AZClient) Delete(ctx context.Context, params AgentPoolParams) error {
	poller, err := c.agentPoolsClient.BeginDelete(
		ctx,
		params.ResourceGroup,
		params.ClusterName,
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

func (c *AZClient) List(ctx context.Context, params AgentPoolParams) ([]*AgentPoolInfo, error) {
	pager := c.agentPoolsClient.NewListPager(
		params.ResourceGroup,
		params.ClusterName,
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

func (c *AZClient) convertToAgentPoolInfo(ap *armcontainerservice.AgentPool) *AgentPoolInfo {
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
		NodeImageVersion:  ap.Properties.NodeImageVersion,
	}
}

// nodeClaimToAgentPool converts NodeClaim to AKS AgentPool
func (c *AZClient) nodeClaimToAgentPool(vmSize string, nodeClaim *karpenterv1.NodeClaim) (armcontainerservice.AgentPool, error) {
	taints := nodeClaim.Spec.Taints
	taintsStr := []*string{}
	for _, t := range taints {
		taintsStr = append(taintsStr, to.Ptr(fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect)))
	}

	scaleSetsType := armcontainerservice.AgentPoolTypeVirtualMachineScaleSets
	labels := map[string]*string{karpenterv1.NodePoolLabelKey: to.Ptr("kaito")}
	for k, v := range nodeClaim.Labels {
		labels[k] = to.Ptr(v)
	}

	if strings.Contains(vmSize, "Standard_N") {
		labels = lo.Assign(labels, map[string]*string{"kaito.sh/machine-type": to.Ptr("gpu")})
	} else {
		labels = lo.Assign(labels, map[string]*string{"kaito.sh/machine-type": to.Ptr("cpu")})
	}

	// Add creation timestamp label
	labels["kaito.sh/creation-timestamp"] = to.Ptr(nodeClaim.CreationTimestamp.UTC().Format("2006-01-02T15-04-05Z"))

	storage := &resource.Quantity{}
	if nodeClaim.Spec.Resources.Requests != nil {
		storage = nodeClaim.Spec.Resources.Requests.Storage()
	}
	var diskSizeGB int32
	if storage.Value() <= 0 {
		return armcontainerservice.AgentPool{}, fmt.Errorf("storage request of nodeclaim(%s) should be more than 0", nodeClaim.Name)
	} else {
		diskSizeGB = int32(storage.Value() >> 30)
	}

	return armcontainerservice.AgentPool{
		Properties: &armcontainerservice.ManagedClusterAgentPoolProfileProperties{
			NodeLabels:   labels,
			NodeTaints:   taintsStr,
			Type:         to.Ptr(scaleSetsType),
			VMSize:       to.Ptr(vmSize),
			OSType:       to.Ptr(armcontainerservice.OSTypeLinux),
			Count:        to.Ptr(int32(1)),
			OSDiskSizeGB: to.Ptr(diskSizeGB),
		},
	}, nil
}

func NewAZClientFromAPI(
	agentPoolsClient AgentPoolsAPI,
) *AZClient {
	return &AZClient{
		agentPoolsClient: agentPoolsClient,
	}
}

func CreateAKSAzClient(cfg *auth.Config) (*AZClient, error) {
	// Defaulting env to Azure Public Cloud.
	env := azure.PublicCloud
	var err error

	azClient, err := NewAKSAZClient(cfg, &env)
	if err != nil {
		return nil, err
	}

	return azClient, nil
}

func NewAKSAZClient(cfg *auth.Config, env *azure.Environment) (*AZClient, error) {
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
