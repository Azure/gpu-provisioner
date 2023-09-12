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

package auth

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
)

const (
	// auth methods
	authMethodPrincipal = "principal"
	authMethodCLI       = "cli"

	// toggle
	dynamicSKUCacheDefault = false
)

const (
	// from azure_manager
	vmTypeVMSS = "vmss"
	vmTypeAKS  = "aks"
)

// ClientConfig contains all essential information to create an Azure client.
type ClientConfig struct {
	CloudName               string
	Location                string
	SubscriptionID          string
	ResourceManagerEndpoint string
	Authorizer              autorest.Authorizer
	UserAgent               string
}

// Config holds the configuration parsed from the --cloud-config flag
type Config struct {
	Cloud          string `json:"cloud" yaml:"cloud"`
	Location       string `json:"location" yaml:"location"`
	TenantID       string `json:"tenantId" yaml:"tenantId"`
	SubscriptionID string `json:"subscriptionId" yaml:"subscriptionId"`
	ResourceGroup  string `json:"resourceGroup" yaml:"resourceGroup"`
	VMType         string `json:"vmType" yaml:"vmType"`

	// AuthMethod determines how to authorize requests for the Azure
	// cloud. Valid options are "principal" (= the traditional
	// service principle approach) and "cli" (= load az command line
	// config file). The default is "principal".
	AuthMethod string `json:"authMethod" yaml:"authMethod"`

	// Settings for a service principal.

	AADClientID                 string `json:"aadClientId" yaml:"aadClientId"`
	AADClientSecret             string `json:"aadClientSecret" yaml:"aadClientSecret"`
	AADClientCertPath           string `json:"aadClientCertPath" yaml:"aadClientCertPath"`
	AADClientCertPassword       string `json:"aadClientCertPassword" yaml:"aadClientCertPassword"`
	UseManagedIdentityExtension bool   `json:"useManagedIdentityExtension" yaml:"useManagedIdentityExtension"`
	UserAssignedIdentityID      string `json:"userAssignedIdentityID" yaml:"userAssignedIdentityID"`

	//Configs only for AKS
	ClusterName string `json:"clusterName" yaml:"clusterName"`
	//Config only for AKS
	NodeResourceGroup string `json:"nodeResourceGroup" yaml:"nodeResourceGroup"`
	//SubnetId is the resource ID of the subnet that VM network interfaces should use
	SubnetID string

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

func (cfg *Config) PrepareConfig() error {
	cfg.BaseVars()
	err := cfg.PrepareSub()
	if err != nil {
		return err
	}
	err = cfg.prepareMSI()
	if err != nil {
		return err
	}
	return nil
}

func (cfg *Config) BaseVars() {
	cfg.Cloud = os.Getenv("ARM_CLOUD")
	cfg.Location = os.Getenv("LOCATION")
	cfg.ResourceGroup = os.Getenv("ARM_RESOURCE_GROUP")
	cfg.TenantID = os.Getenv("ARM_TENANT_ID")
	cfg.AADClientID = os.Getenv("ARM_CLIENT_ID")
	cfg.VMType = strings.ToLower(os.Getenv("ARM_VM_TYPE"))
	cfg.AADClientCertPath = os.Getenv("ARM_CLIENT_CERT_PATH")
	cfg.AADClientCertPassword = os.Getenv("ARM_CLIENT_CERT_PASSWORD")
	cfg.ClusterName = os.Getenv("AZURE_CLUSTER_NAME")
	cfg.NodeResourceGroup = os.Getenv("AZURE_NODE_RESOURCE_GROUP")
	cfg.SubnetID = os.Getenv("AZURE_SUBNET_ID")
}
func (cfg *Config) PrepareSub() error {
	subscriptionID := getSubscriptionIDFromInstanceMetadata()
	if subscriptionID == "" {
		return fmt.Errorf("ARM_SUBSCRIPTION_ID is not set as an environment variable")
	}
	cfg.SubscriptionID = subscriptionID
	return nil
}

func (cfg *Config) prepareMSI() error {
	useManagedIdentityExtensionFromEnv := os.Getenv("ARM_USE_MANAGED_IDENTITY_EXTENSION")
	if len(useManagedIdentityExtensionFromEnv) > 0 {
		shouldUse, err := strconv.ParseBool(useManagedIdentityExtensionFromEnv)
		if err != nil {
			return err
		}
		cfg.UseManagedIdentityExtension = shouldUse
	}
	userAssignedIdentityIDFromEnv := os.Getenv("ARM_USER_ASSIGNED_IDENTITY_ID")
	if userAssignedIdentityIDFromEnv != "" {
		cfg.UserAssignedIdentityID = userAssignedIdentityIDFromEnv
	}
	return nil
}

// BuildAzureConfig returns a Config object for the Azure clients
// TODO: remove nolint on gocyclo. Added for now in order to pass "make verify" in azure/poc
// nolint: gocyclo
func BuildAzureConfig() (*Config, error) {
	var err error
	cfg := &Config{}
	err = cfg.PrepareConfig()
	if err != nil {
		return nil, err
	}
	if enableDynamicSKUCache := os.Getenv("AZURE_ENABLE_DYNAMIC_SKU_CACHE"); enableDynamicSKUCache != "" {
		cfg.EnableDynamicSKUCache, err = strconv.ParseBool(enableDynamicSKUCache)
		if err != nil {
			return nil, fmt.Errorf("failed to parse AZURE_ENABLE_DYNAMIC_SKU_CACHE %q: %w", enableDynamicSKUCache, err)
		}
	} else {
		cfg.EnableDynamicSKUCache = dynamicSKUCacheDefault
	}

	cfg.TrimSpace()

	// Defaulting vmType to vmss.
	if cfg.VMType == "" {
		cfg.VMType = vmTypeVMSS
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (cfg *Config) GetAzureClientConfig(authorizer autorest.Authorizer, env *azure.Environment) *ClientConfig {
	azClientConfig := &ClientConfig{
		Location:                cfg.Location,
		SubscriptionID:          cfg.SubscriptionID,
		ResourceManagerEndpoint: env.ResourceManagerEndpoint,
		Authorizer:              authorizer,
	}

	return azClientConfig
}

// TrimSpace removes all leading and trailing white spaces.
func (cfg *Config) TrimSpace() {
	cfg.Cloud = strings.TrimSpace(cfg.Cloud)
	cfg.TenantID = strings.TrimSpace(cfg.TenantID)
	cfg.SubscriptionID = strings.TrimSpace(cfg.SubscriptionID)
	cfg.ResourceGroup = strings.TrimSpace(cfg.ResourceGroup)
	cfg.VMType = strings.TrimSpace(cfg.VMType)
	cfg.AADClientID = strings.TrimSpace(cfg.AADClientID)
	cfg.AADClientSecret = strings.TrimSpace(cfg.AADClientSecret)
	cfg.AADClientCertPath = strings.TrimSpace(cfg.AADClientCertPath)
	cfg.AADClientCertPassword = strings.TrimSpace(cfg.AADClientCertPassword)
	cfg.ClusterName = strings.TrimSpace(cfg.ClusterName)
	cfg.NodeResourceGroup = strings.TrimSpace(cfg.NodeResourceGroup)
}

// TODO: remove nolint on gocyclo. Added for now in order to pass "make verify" in azure/poc
// nolint: gocyclo
func (cfg *Config) validate() error {
	if cfg.VMType == vmTypeAKS {
		// Cluster name is a mandatory param to proceed.
		if cfg.ClusterName == "" {
			return fmt.Errorf("cluster name not set for type %+v", cfg.VMType)
		}
	}

	if cfg.SubscriptionID == "" {
		return fmt.Errorf("subscription ID not set")
	}

	if cfg.UseManagedIdentityExtension {
		return nil
	}

	if cfg.TenantID == "" {
		return fmt.Errorf("tenant ID not set")
	}

	switch cfg.AuthMethod {
	case "", authMethodPrincipal:
		if cfg.AADClientID == "" {
			return errors.New("ARM Client ID not set")
		}
	case authMethodCLI:
		// Nothing to check at the moment.
	default:
		return fmt.Errorf("unsupported authorization method: %s", cfg.AuthMethod)
	}

	if cfg.NodeResourceGroup == "" {
		return fmt.Errorf("node resource group is not set")
	}

	if cfg.SubnetID == "" {
		return fmt.Errorf("subnet ID is not set")
	}

	return nil
}

// getSubscriptionId reads the Subscription ID from the instance metadata.
func getSubscriptionIDFromInstanceMetadata() string {
	subscriptionID, _ := os.LookupEnv("ARM_SUBSCRIPTION_ID")
	return subscriptionID
}
