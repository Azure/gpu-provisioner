#!/bin/bash

# Example script showing how to deploy a KAITO workspace with Azure Linux nodes
# This script demonstrates the new Azure Linux support in GPU Provisioner

set -e

echo "🚀 Deploying KAITO Workspace with Azure Linux support..."

# Create a NodeClaim that requests Azure Linux
cat <<EOF | kubectl apply -f -
apiVersion: karpenter.sh/v1
kind: NodeClaim
metadata:
  name: azure-linux-gpu-node
  labels:
    kaito.sh/node-image-family: "AzureLinux"
    kaito.sh/workspace: "llama-workspace-azurelinux"
spec:
  requirements:
    - key: node.kubernetes.io/instance-type
      operator: In
      values: ["Standard_NC6s_v3"]
    - key: karpenter.sh/capacity-type
      operator: In
      values: ["on-demand"]
  resources:
    requests:
      storage: "50Gi"
  # Note: NodeClassRef can be added here if using karpenter-provider-azure directly
  # nodeClassRef:
  #   apiVersion: karpenter.azure.com/v1alpha1
  #   kind: AKSNodeClass
  #   name: default
EOF

echo "✅ NodeClaim created with Azure Linux image family"

# Wait for the node to be ready
echo "⏳ Waiting for node to be provisioned..."
kubectl wait --for=condition=Ready nodeclaim/azure-linux-gpu-node --timeout=600s

# Create a KAITO workspace that will use the Azure Linux node
cat <<EOF | kubectl apply -f -
apiVersion: kaito.ai/v1alpha1
kind: Workspace
metadata:
  name: llama-workspace-azurelinux
spec:
  resource:
    instanceType: "Standard_NC6s_v3"
    labelSelector:
      matchLabels:
        kaito.sh/workspace: "llama-workspace-azurelinux"
        kaito.sh/node-image-family: "AzureLinux"
  inference:
    preset:
      name: llama-7b
EOF

echo "✅ KAITO Workspace created"

# Wait for the workspace to be ready
echo "⏳ Waiting for KAITO workspace to be ready..."
kubectl wait --for=condition=WorkspaceReady workspace/llama-workspace-azurelinux --timeout=900s

# Show the status
echo "📊 Checking deployment status..."
echo ""
echo "NodeClaim status:"
kubectl get nodeclaim azure-linux-gpu-node -o wide

echo ""
echo "Node details:"
kubectl get nodes -l kaito.sh/workspace=llama-workspace-azurelinux -o custom-columns=NAME:.metadata.name,OS-IMAGE:.status.nodeInfo.osImage,INSTANCE-TYPE:.metadata.labels.node\\.kubernetes\\.io/instance-type

echo ""
echo "KAITO Workspace status:"
kubectl get workspace llama-workspace-azurelinux -o wide

echo ""
echo "🎉 Azure Linux GPU node with KAITO workspace deployed successfully!"
echo ""
echo "To verify the OS SKU in Azure:"
echo "az aks agentpool list --resource-group <your-rg> --cluster-name <your-cluster> --query '[?name==\`azure-linux-gpu-node\`].{Name:name,OsSku:osSku,VmSize:vmSize}' -o table"
