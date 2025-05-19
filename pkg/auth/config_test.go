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
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setEnvVars(vars map[string]string) {
	for k, v := range vars {
		_ = os.Setenv(k, v)
	}
}

func unsetEnvVars(keys []string) {
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
}

func TestBaseVars(t *testing.T) {
	envs := map[string]string{
		"LOCATION":            "eastus",
		"ARM_RESOURCE_GROUP":  "test-rg",
		"AZURE_TENANT_ID":     "tenant-123",
		"AZURE_CLIENT_ID":     "client-456",
		"AZURE_CLUSTER_NAME":  "cluster-789",
		"ARM_SUBSCRIPTION_ID": "sub-abc",
		"DEPLOYMENT_MODE":     "mode1",
		"CLOUD_ENVIRONMENT":   "AzurePublicCloud",
	}
	setEnvVars(envs)
	defer unsetEnvVars([]string{
		"LOCATION", "ARM_RESOURCE_GROUP", "AZURE_TENANT_ID", "AZURE_CLIENT_ID",
		"AZURE_CLUSTER_NAME", "ARM_SUBSCRIPTION_ID", "DEPLOYMENT_MODE", "CLOUD_ENVIRONMENT",
	})

	cfg := &Config{}
	cfg.BaseVars()

	assert.Equal(t, "eastus", cfg.Location)
	assert.Equal(t, "test-rg", cfg.ResourceGroup)
	assert.Equal(t, "tenant-123", cfg.TenantID)
	assert.Equal(t, "client-456", cfg.UserAssignedIdentityID)
	assert.Equal(t, "cluster-789", cfg.ClusterName)
	assert.Equal(t, "sub-abc", cfg.SubscriptionID)
	assert.Equal(t, "mode1", cfg.DeploymentMode)
	assert.Equal(t, "AzurePublicCloud", cfg.CloudEnvironment)
}

func TestBuildAzureConfig_EnableDynamicSKUCache(t *testing.T) {
	envs := map[string]string{
		"ARM_SUBSCRIPTION_ID":            "sub-abc",
		"AZURE_TENANT_ID":                "tenant-123",
		"AZURE_ENABLE_DYNAMIC_SKU_CACHE": "true",
	}
	setEnvVars(envs)
	defer unsetEnvVars([]string{
		"ARM_SUBSCRIPTION_ID", "AZURE_TENANT_ID", "AZURE_ENABLE_DYNAMIC_SKU_CACHE",
	})

	cfg, err := BuildAzureConfig()
	assert.NoError(t, err)
	assert.True(t, cfg.EnableDynamicSKUCache)
}

func TestBuildAzureConfig_InvalidDynamicSKUCache(t *testing.T) {
	envs := map[string]string{
		"ARM_SUBSCRIPTION_ID":            "sub-abc",
		"AZURE_TENANT_ID":                "tenant-123",
		"AZURE_ENABLE_DYNAMIC_SKU_CACHE": "notabool",
	}
	setEnvVars(envs)
	defer unsetEnvVars([]string{
		"ARM_SUBSCRIPTION_ID", "AZURE_TENANT_ID", "AZURE_ENABLE_DYNAMIC_SKU_CACHE",
	})

	_, err := BuildAzureConfig()
	assert.Error(t, err)
}

func TestBuildAzureConfig_MissingRequired(t *testing.T) {
	envs := map[string]string{}
	setEnvVars(envs)
	defer unsetEnvVars([]string{
		"ARM_SUBSCRIPTION_ID", "AZURE_TENANT_ID",
	})

	_, err := BuildAzureConfig()
	assert.Error(t, err)
}

func TestTrimSpace(t *testing.T) {
	cfg := &Config{
		TenantID:       " tenant ",
		SubscriptionID: " sub ",
		ResourceGroup:  " rg ",
		ClusterName:    " cluster ",
	}
	cfg.TrimSpace()
	assert.Equal(t, "tenant", cfg.TenantID)
	assert.Equal(t, "sub", cfg.SubscriptionID)
	assert.Equal(t, "rg", cfg.ResourceGroup)
	assert.Equal(t, "cluster", cfg.ClusterName)
}

func TestValidate(t *testing.T) {
	cfg := &Config{
		TenantID:       "tenant",
		SubscriptionID: "sub",
	}
	assert.NoError(t, cfg.validate())

	cfg.TenantID = ""
	assert.Error(t, cfg.validate())

	cfg.TenantID = "tenant"
	cfg.SubscriptionID = ""
	assert.Error(t, cfg.validate())
}

func TestBuildAzureConfig_DefaultDynamicSKUCache(t *testing.T) {
	envs := map[string]string{
		"ARM_SUBSCRIPTION_ID": "sub-abc",
		"AZURE_TENANT_ID":     "tenant-123",
	}
	setEnvVars(envs)
	defer unsetEnvVars([]string{
		"ARM_SUBSCRIPTION_ID", "AZURE_TENANT_ID", "AZURE_ENABLE_DYNAMIC_SKU_CACHE",
	})

	cfg, err := BuildAzureConfig()
	assert.NoError(t, err)
	assert.Equal(t, dynamicSKUCacheDefault, cfg.EnableDynamicSKUCache)
}

func TestConfig_GetAzureClientConfig(t *testing.T) {
	cfg := &Config{
		Location:       "eastus",
		SubscriptionID: "sub-abc",
	}
	clientCfg := cfg.GetAzureClientConfig(nil, "resourceEndpoint")
	assert.Equal(t, "eastus", clientCfg.Location)
	assert.Equal(t, "sub-abc", clientCfg.SubscriptionID)
}

func TestConfigureHTTP2Transport(t *testing.T) {
	transport := &http.Transport{
		ForceAttemptHTTP2: true,
	}
	err := configureHTTP2Transport(transport)
	assert.NoError(t, err)
}
