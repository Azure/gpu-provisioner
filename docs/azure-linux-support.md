# Azure Linux Support in GPU Provisioner

This document describes how to use Azure Linux nodes with KAITO and the GPU Provisioner.

## Overview

The GPU Provisioner now supports provisioning Azure Linux nodes for KAITO workloads. Azure Linux is a container-optimized Linux distribution built by Microsoft that provides enhanced security, performance, and reliability for containerized workloads.

## Configuration

To request Azure Linux nodes, you can specify the desired OS image family using either labels or annotations on your NodeClaim.

### Using Labels (Recommended)

Add the `kaito.sh/node-image-family` label to your NodeClaim:

```yaml
apiVersion: karpenter.sh/v1
kind: NodeClaim
metadata:
  name: azurelinux-node
  labels:
    kaito.sh/node-image-family: "AzureLinux"
spec:
  requirements:
    - key: node.kubernetes.io/instance-type
      operator: In
      values: ["Standard_NC6s_v3"]
  resources:
    requests:
      storage: "30Gi"
```

### Using Annotations

Alternatively, you can use annotations:

```yaml
apiVersion: karpenter.sh/v1
kind: NodeClaim
metadata:
  name: azurelinux-node
  annotations:
    kaito.sh/node-image-family: "AzureLinux"
spec:
  requirements:
    - key: node.kubernetes.io/instance-type
      operator: In
      values: ["Standard_NC6s_v3"]
  resources:
    requests:
      storage: "30Gi"
```

## Supported Image Families

| Image Family | OS SKU | Description |
|-------------|--------|-------------|
| `AzureLinux` | AzureLinux | Container-optimized Linux distribution by Microsoft |
| `Ubuntu` | Ubuntu | Standard Ubuntu distribution |
| `Ubuntu2204` | Ubuntu | Ubuntu 22.04 LTS |

## Default Behavior

- If no `kaito.sh/node-image-family` is specified, the system defaults to Ubuntu
- Image family names are case-insensitive
- Labels take precedence over annotations if both are specified
- Unknown image family values default to Ubuntu with a warning

## Examples

### KAITO Workspace with Azure Linux

```yaml
apiVersion: kaito.ai/v1alpha1
kind: Workspace
metadata:
  name: workspace-llama-7b-azurelinux
spec:
  resource:
    instanceType: "Standard_NC6s_v3"
    labelSelector:
      matchLabels:
        kaito.sh/node-image-family: "AzureLinux"
  inference:
    preset:
      name: llama-7b
```

### NodeClaim Template for Azure Linux

```yaml
apiVersion: karpenter.sh/v1
kind: NodeClaim
metadata:
  generateName: kaito-azurelinux-
  labels:
    kaito.sh/node-image-family: "AzureLinux"
    kaito.sh/workspace: "my-workspace"
spec:
  requirements:
    - key: node.kubernetes.io/instance-type
      operator: In
      values: ["Standard_NC6s_v3", "Standard_NC12s_v3"]
    - key: karpenter.sh/capacity-type
      operator: In
      values: ["spot", "on-demand"]
  resources:
    requests:
      storage: "50Gi"
  nodeClassRef:
    apiVersion: karpenter.azure.com/v1alpha1
    kind: AKSNodeClass
    name: default
```

## Migration from Ubuntu to Azure Linux

To migrate existing workloads from Ubuntu to Azure Linux:

1. Update your NodeClaim or Workspace specifications to include the Azure Linux label
2. Test your applications on Azure Linux nodes to ensure compatibility
3. Gradually migrate workloads by updating your KAITO Workspace configurations

## Benefits of Azure Linux

- **Security**: Enhanced security features and reduced attack surface
- **Performance**: Optimized for containerized workloads
- **Reliability**: Designed for cloud-native applications
- **Size**: Smaller footprint compared to traditional distributions
- **Support**: Backed by Microsoft support

## Troubleshooting

### Common Issues

1. **Unknown image family warning**: Ensure the image family name is spelled correctly and is one of the supported values
2. **Default to Ubuntu**: Check that your label/annotation key is exactly `kaito.sh/node-image-family`
3. **Case sensitivity**: Image family values are case-insensitive, so `azurelinux`, `AzureLinux`, and `AZURELINUX` all work

### Verification

To verify that your nodes are using Azure Linux:

```bash
# Check the OS image on your nodes
kubectl get nodes -o custom-columns=NAME:.metadata.name,OS-IMAGE:.status.nodeInfo.osImage

# Check agent pool configuration
az aks agentpool show --resource-group <rg> --cluster-name <cluster> --name <pool-name> --query "osSkU"
```

## Implementation Details

The GPU Provisioner determines the OS SKU for Azure Kubernetes Service (AKS) agent pools based on the `kaito.sh/node-image-family` label or annotation on the NodeClaim. The implementation:

1. Checks for the label first (takes precedence)
2. Falls back to checking annotations
3. Maps the image family to the appropriate AKS OS SKU
4. Defaults to Ubuntu if no valid image family is specified

This approach ensures compatibility with existing KAITO workloads while enabling new deployments to take advantage of Azure Linux.
