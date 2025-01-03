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
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/go-autorest/autorest/azure"
)

// Config holds the configuration parsed from the --cloud-config flag
type Config struct {
	Cloud                  string `json:"cloud" yaml:"cloud"`
	Location               string `json:"location" yaml:"location"`
	TenantID               string `json:"tenantId" yaml:"tenantId"`
	SubscriptionID         string `json:"subscriptionId" yaml:"subscriptionId"`
	ResourceGroup          string `json:"resourceGroup" yaml:"resourceGroup"`
	UserAssignedIdentityID string `json:"userAssignedIdentityID" yaml:"userAssignedIdentityID"`

	//Configs only for AKS
	ClusterName       string `json:"clusterName" yaml:"clusterName"`
	NodeResourceGroup string `json:"nodeResourceGroup" yaml:"nodeResourceGroup"`
}

func (cfg *Config) BaseVars() {
	cfg.Cloud = os.Getenv("ARM_CLOUD")
	if cfg.Cloud == "" {
		cfg.Cloud = azure.PublicCloud.Name
	}
	cfg.Location = os.Getenv("LOCATION")
	cfg.ResourceGroup = os.Getenv("ARM_RESOURCE_GROUP")
	cfg.TenantID = os.Getenv("AZURE_TENANT_ID")
	cfg.UserAssignedIdentityID = os.Getenv("AZURE_CLIENT_ID")
	cfg.ClusterName = os.Getenv("AZURE_CLUSTER_NAME")
	cfg.NodeResourceGroup = os.Getenv("AZURE_NODE_RESOURCE_GROUP")
	cfg.SubscriptionID = os.Getenv("ARM_SUBSCRIPTION_ID")
}

// BuildAzureConfig returns a Config object for the Azure clients
func BuildAzureConfig() (*Config, error) {
	cfg := &Config{}
	cfg.BaseVars()
	cfg.TrimSpace()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// TrimSpace removes all leading and trailing white spaces.
func (cfg *Config) TrimSpace() {
	cfg.Cloud = strings.ToLower(strings.TrimSpace(cfg.Cloud))
	cfg.TenantID = strings.TrimSpace(cfg.TenantID)
	cfg.SubscriptionID = strings.TrimSpace(cfg.SubscriptionID)
	cfg.ResourceGroup = strings.TrimSpace(cfg.ResourceGroup)
	cfg.ClusterName = strings.TrimSpace(cfg.ClusterName)
	cfg.NodeResourceGroup = strings.TrimSpace(cfg.NodeResourceGroup)
}

// nolint: gocyclo
func (cfg *Config) validate() error {
	if cfg.SubscriptionID == "" {
		return fmt.Errorf("subscription ID not set")
	}
	if cfg.TenantID == "" {
		return fmt.Errorf("tenant ID not set")
	}
	if cfg.NodeResourceGroup == "" {
		return fmt.Errorf("node resource group is not set")
	}

	return nil
}

type CloudEnvironmentName string

const (
	AzurePublicCloud       CloudEnvironmentName = "azurepubliccloud"
	AzureUSGovernmentCloud CloudEnvironmentName = "azureusgovernmentcloud"
	AzureChinaCloud        CloudEnvironmentName = "azurechinacloud"
)

func (cfg *Config) getCloudConfiguration() cloud.Configuration {
	switch cfg.Cloud {
	case string(AzurePublicCloud):
		return cloud.AzurePublic
	case string(AzureUSGovernmentCloud):
		return cloud.AzureGovernment
	case string(AzureChinaCloud):
		return cloud.AzureChina
	}
	panic("cloud config for cloud name " + cfg.Cloud + " does not exist")
}
