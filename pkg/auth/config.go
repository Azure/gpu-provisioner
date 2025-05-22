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

package auth

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/azure/gpu-provisioner/pkg/utils"
)

const (
	// toggle
	dynamicSKUCacheDefault          = false
	E2E_RP_INGRESS_ENDPOINT         = "rp.e2e.ig.e2e-aks.azure.com"
	HTTPS_PORT                      = ":443"
	E2E_RP_INGRESS_ENDPOINT_ADDRESS = E2E_RP_INGRESS_ENDPOINT + HTTPS_PORT
	// if no frame is received for 30s, the transport will issue a ping health check to the server.
	http2ReadIdleTimeout = 30 * time.Second

	// we give 10s to the server to respond to the ping. if no response is received,
	// the transport will close the connection, so that the next request will open a new connection and not
	// hit a context deadline exceeded error.
	http2PingTimeout = 10 * time.Second

	// e2e environment variables
	e2eOverlayResourceVersionKey = "AKS_E2E_OVERLAY_RESOURCE_VERSION"
	e2eBuildVersion              = "AKS_E2E_BUILD_VERSION"
)

// ClientConfig contains all essential information to create an Azure client.
type ClientConfig struct {
	CloudName      string
	Location       string
	SubscriptionID string
	Authorizer     autorest.Authorizer
	UserAgent      string
}

// Config holds the configuration parsed from the --cloud-config flag
type Config struct {
	Location               string `json:"location" yaml:"location"`
	TenantID               string `json:"tenantId" yaml:"tenantId"`
	SubscriptionID         string `json:"subscriptionId" yaml:"subscriptionId"`
	ResourceGroup          string `json:"resourceGroup" yaml:"resourceGroup"`
	DeploymentMode         string `json:"deploymentMode" yaml:"deploymentMode"`
	CloudEnvironment       string `json:"cloudEnvironment" yaml:"cloudEnvironment"`
	UserAssignedIdentityID string `json:"userAssignedIdentityID" yaml:"userAssignedIdentityID"`

	// e2e configuration
	E2EBuildVersion           string `json:"e2eBuildVersion" yaml:"e2eBuildVersion"`
	E2EOverlayResourceVersion string `json:"overlayResourceVersion" yaml:"overlayResourceVersion"`

	//Configs only for AKS
	ClusterName string `json:"clusterName" yaml:"clusterName"`
	// enableDynamicSKUCache defines whether to enable dynamic instance workflow for instance information check
	EnableDynamicSKUCache bool `json:"enableDynamicSKUCache,omitempty" yaml:"enableDynamicSKUCache,omitempty"`
	// EnableDetailedCSEMessage defines whether to emit error messages in the CSE error body info
	EnableDetailedCSEMessage bool `json:"enableDetailedCSEMessage,omitempty" yaml:"enableDetailedCSEMessage,omitempty"`

	// EnableForceDelete defines whether to enable force deletion on the APIs
	EnableForceDelete bool `json:"enableForceDelete,omitempty" yaml:"enableForceDelete,omitempty"`

	// EnableGetVmss defines whether to enable making a call to GET VMSS to fetch fresh capacity info
	// The TTL for this cache is controlled by the GetVmssSizeRefreshPeriod interval
	EnableGetVmss bool `json:"enableGetVmss,omitempty" yaml:"enableGetVmss,omitempty"`

	// GetVmssSizeRefreshPeriod defines how frequently to call GET VMSS API to fetch VMSS info per nodegroup instance
	GetVmssSizeRefreshPeriod time.Duration `json:"getVmssSizeRefreshPeriod,omitempty" yaml:"getVmssSizeRefreshPeriod,omitempty"`

	// EnablePartialScaling defines whether to enable partial scaling based on quota limits
	EnablePartialScaling bool `json:"enablePartialScaling,omitempty" yaml:"enablePartialScaling,omitempty"`
}

func (cfg *Config) BaseVars() {
	cfg.Location = os.Getenv("LOCATION")
	cfg.ResourceGroup = os.Getenv("ARM_RESOURCE_GROUP")
	cfg.TenantID = os.Getenv("AZURE_TENANT_ID")
	cfg.UserAssignedIdentityID = os.Getenv("AZURE_CLIENT_ID")
	cfg.ClusterName = os.Getenv("AZURE_CLUSTER_NAME")
	cfg.SubscriptionID = os.Getenv("ARM_SUBSCRIPTION_ID")
	cfg.DeploymentMode = os.Getenv("DEPLOYMENT_MODE")
	cfg.CloudEnvironment = os.Getenv("CLOUD_ENVIRONMENT")

	e2eMode := utils.WithDefaultBool("E2E_TEST_MODE", false)
	if e2eMode {
		cfg.E2EOverlayResourceVersion = os.Getenv(e2eOverlayResourceVersionKey)
		cfg.E2EBuildVersion = os.Getenv(e2eBuildVersion)
	}
}

// BuildAzureConfig returns a Config object for the Azure clients
// nolint: gocyclo
func BuildAzureConfig() (*Config, error) {
	var err error
	cfg := &Config{}
	cfg.BaseVars()
	if enableDynamicSKUCache := os.Getenv("AZURE_ENABLE_DYNAMIC_SKU_CACHE"); enableDynamicSKUCache != "" {
		cfg.EnableDynamicSKUCache, err = strconv.ParseBool(enableDynamicSKUCache)
		if err != nil {
			return nil, fmt.Errorf("failed to parse AZURE_ENABLE_DYNAMIC_SKU_CACHE %q: %w", enableDynamicSKUCache, err)
		}
	} else {
		cfg.EnableDynamicSKUCache = dynamicSKUCacheDefault
	}

	cfg.TrimSpace()

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (cfg *Config) GetAzureClientConfig(authorizer autorest.Authorizer, resourceEndpoint string) *ClientConfig {
	azClientConfig := &ClientConfig{
		Location:       cfg.Location,
		SubscriptionID: cfg.SubscriptionID,
		Authorizer:     authorizer,
	}

	return azClientConfig
}

// TrimSpace removes all leading and trailing white spaces.
func (cfg *Config) TrimSpace() {
	cfg.TenantID = strings.TrimSpace(cfg.TenantID)
	cfg.SubscriptionID = strings.TrimSpace(cfg.SubscriptionID)
	cfg.ResourceGroup = strings.TrimSpace(cfg.ResourceGroup)
	cfg.ClusterName = strings.TrimSpace(cfg.ClusterName)
}

// nolint: gocyclo
func (cfg *Config) validate() error {
	if cfg.SubscriptionID == "" {
		return fmt.Errorf("subscription ID not set")
	}
	if cfg.TenantID == "" {
		return fmt.Errorf("tenant ID not set")
	}

	return nil
}
