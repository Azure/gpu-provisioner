#!/usr/bin/env bash
set -euo pipefail

# https://github.com/Azure/karpenter-provider-azure/blob/2beb773cbd3134eeabb8c96b72a130b86b1a91e1/hack/deploy/configure-values.sh

# This script interrogates the AKS cluster and Azure resources to generate
# the gpu-provisioner-values.yaml file using the gpu-provisioner-values-template.yaml file as a template.

if [ "$#" -ne 3 ]; then
    echo "Usage: $0 <cluster-name> <resource-group> <gpu-provisioner-user-assigned-identity-name>"
    exit 1
fi

echo "Configuring gpu-provisioner-values.yaml for cluster $1 in resource group $2 ..."

CLUSTER_NAME=$1
AZURE_RESOURCE_GROUP=$2
AZURE_GPU_PROVISIONER_USER_ASSIGNED_IDENTITY_NAME=$3

AKS_JSON=$(az aks show --name "$CLUSTER_NAME" --resource-group "$AZURE_RESOURCE_GROUP" -o json)
AZURE_LOCATION=$(jq -r ".location" <<< "$AKS_JSON")
AZURE_RESOURCE_GROUP_MC=$(jq -r ".nodeResourceGroup" <<< "$AKS_JSON")
AZURE_TENANT_ID=$(az account show -o json |jq -r ".tenantId")
AZURE_SUBSCRIPTION_ID=$(az account show -o json |jq -r ".id")

GPU_PROVISIONER_USER_ASSIGNED_CLIENT_ID=$(az identity show --resource-group "${AZURE_RESOURCE_GROUP}" --name "${AZURE_GPU_PROVISIONER_USER_ASSIGNED_IDENTITY_NAME}" --query 'clientId' -otsv)

export CLUSTER_NAME AZURE_LOCATION AZURE_RESOURCE_GROUP AZURE_RESOURCE_GROUP_MC GPU_PROVISIONER_USER_ASSIGNED_CLIENT_ID AZURE_TENANT_ID AZURE_SUBSCRIPTION_ID

# get gpu-provisioner-values-template.yaml, if not already present (e.g. outside of repo context)
if [ ! -f gpu-provisioner-values-template.yaml ]; then
    curl -sO https://raw.githubusercontent.com/Azure/gpu-provisioner/main/gpu-provisioner-values-template.yaml
fi
yq '(.. | select(tag == "!!str")) |= envsubst' gpu-provisioner-values-template.yaml > gpu-provisioner-values.yaml
