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
	"os"
	"testing"

	"github.com/Azure/go-autorest/autorest/azure"
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
	}
	setEnvVars(envs)
	defer unsetEnvVars([]string{
		"LOCATION", "ARM_RESOURCE_GROUP", "AZURE_TENANT_ID", "AZURE_CLIENT_ID",
		"AZURE_CLUSTER_NAME", "ARM_SUBSCRIPTION_ID", "DEPLOYMENT_MODE",
	})

	cfg := &Config{}
	cfg.BaseVars()

	if cfg.Location != "eastus" {
		t.Errorf("expected Location to be 'eastus', got %s", cfg.Location)
	}
	if cfg.ResourceGroup != "test-rg" {
		t.Errorf("expected ResourceGroup to be 'test-rg', got %s", cfg.ResourceGroup)
	}
	if cfg.TenantID != "tenant-123" {
		t.Errorf("expected TenantID to be 'tenant-123', got %s", cfg.TenantID)
	}
	if cfg.UserAssignedIdentityID != "client-456" {
		t.Errorf("expected UserAssignedIdentityID to be 'client-456', got %s", cfg.UserAssignedIdentityID)
	}
	if cfg.ClusterName != "cluster-789" {
		t.Errorf("expected ClusterName to be 'cluster-789', got %s", cfg.ClusterName)
	}
	if cfg.SubscriptionID != "sub-abc" {
		t.Errorf("expected SubscriptionID to be 'sub-abc', got %s", cfg.SubscriptionID)
	}
	if cfg.DeploymentMode != "mode1" {
		t.Errorf("expected DeploymentMode to be 'mode1', got %s", cfg.DeploymentMode)
	}
}

func TestBuildAzureConfig_EnableDynamicSKUCache(t *testing.T) {
	os.Setenv("ARM_SUBSCRIPTION_ID", "sub-abc")
	os.Setenv("AZURE_TENANT_ID", "tenant-123")
	os.Setenv("AZURE_ENABLE_DYNAMIC_SKU_CACHE", "true")
	defer unsetEnvVars([]string{"ARM_SUBSCRIPTION_ID", "AZURE_TENANT_ID", "AZURE_ENABLE_DYNAMIC_SKU_CACHE"})

	cfg, err := BuildAzureConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.EnableDynamicSKUCache {
		t.Errorf("expected EnableDynamicSKUCache to be true")
	}
}

func TestBuildAzureConfig_InvalidDynamicSKUCache(t *testing.T) {
	os.Setenv("ARM_SUBSCRIPTION_ID", "sub-abc")
	os.Setenv("AZURE_TENANT_ID", "tenant-123")
	os.Setenv("AZURE_ENABLE_DYNAMIC_SKU_CACHE", "notabool")
	defer unsetEnvVars([]string{"ARM_SUBSCRIPTION_ID", "AZURE_TENANT_ID", "AZURE_ENABLE_DYNAMIC_SKU_CACHE"})

	_, err := BuildAzureConfig()
	if err == nil {
		t.Errorf("expected error for invalid AZURE_ENABLE_DYNAMIC_SKU_CACHE")
	}
}

func TestBuildAzureConfig_MissingRequired(t *testing.T) {
	os.Unsetenv("ARM_SUBSCRIPTION_ID")
	os.Unsetenv("AZURE_TENANT_ID")

	_, err := BuildAzureConfig()
	if err == nil {
		t.Errorf("expected error for missing required env vars")
	}
}

func TestTrimSpace(t *testing.T) {
	cfg := &Config{
		TenantID:       " tenant ",
		SubscriptionID: " sub ",
		ResourceGroup:  " rg ",
		ClusterName:    " cluster ",
	}
	cfg.TrimSpace()
	if cfg.TenantID != "tenant" {
		t.Errorf("expected TenantID to be 'tenant', got '%s'", cfg.TenantID)
	}
	if cfg.SubscriptionID != "sub" {
		t.Errorf("expected SubscriptionID to be 'sub', got '%s'", cfg.SubscriptionID)
	}
	if cfg.ResourceGroup != "rg" {
		t.Errorf("expected ResourceGroup to be 'rg', got '%s'", cfg.ResourceGroup)
	}
	if cfg.ClusterName != "cluster" {
		t.Errorf("expected ClusterName to be 'cluster', got '%s'", cfg.ClusterName)
	}
}

func TestValidate(t *testing.T) {
	cfg := &Config{
		TenantID:       "tenant",
		SubscriptionID: "sub",
	}
	if err := cfg.validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	cfg.TenantID = ""
	if err := cfg.validate(); err == nil {
		t.Errorf("expected error for missing TenantID")
	}

	cfg.TenantID = "tenant"
	cfg.SubscriptionID = ""
	if err := cfg.validate(); err == nil {
		t.Errorf("expected error for missing SubscriptionID")
	}
}

func TestBuildAzureConfig_DefaultDynamicSKUCache(t *testing.T) {
	os.Setenv("ARM_SUBSCRIPTION_ID", "sub-abc")
	os.Setenv("AZURE_TENANT_ID", "tenant-123")
	os.Unsetenv("AZURE_ENABLE_DYNAMIC_SKU_CACHE")
	defer unsetEnvVars([]string{"ARM_SUBSCRIPTION_ID", "AZURE_TENANT_ID", "AZURE_ENABLE_DYNAMIC_SKU_CACHE"})

	cfg, err := BuildAzureConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.EnableDynamicSKUCache != dynamicSKUCacheDefault {
		t.Errorf("expected EnableDynamicSKUCache to be %v", dynamicSKUCacheDefault)
	}
}

func TestConfig_GetAzureClientConfig(t *testing.T) {
	cfg := &Config{
		Location:       "eastus",
		SubscriptionID: "sub-abc",
	}
	env := azure.PublicCloud
	clientCfg := cfg.GetAzureClientConfig(nil, &env)
	if clientCfg.Location != "eastus" {
		t.Errorf("expected Location to be 'eastus', got %s", clientCfg.Location)
	}
	if clientCfg.SubscriptionID != "sub-abc" {
		t.Errorf("expected SubscriptionID to be 'sub-abc', got %s", clientCfg.SubscriptionID)
	}
}
