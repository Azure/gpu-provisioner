# Azure Linux Support Examples for GPU Provisioner

This document provides examples of how to use Azure Linux nodes with the GPU Provisioner and KAITO.

## Overview

The GPU Provisioner supports Azure Linux nodes through the `kaito.sh/node-image-family` annotation on NodeClaim resources. When this is set to `AzureLinux`, the provisioner will create AKS agent pools with the Azure Linux OS SKU.

## Supported Image Families

| Image Family | OS SKU | Description |
|-------------|--------|-------------|
| `AzureLinux` | AzureLinux | Container-optimized Linux distribution by Microsoft |
| `Ubuntu` | Ubuntu | Standard Ubuntu distribution (default) |

## Configuration Methods

### Method 1: Using Annotations (Required)

```yaml
apiVersion: karpenter.sh/v1
kind: NodeClaim
metadata:
  name: azure-linux-gpu-node
  labels:
    kaito.sh/workspace: "my-workspace"
  annotations:
    kaito.sh/node-image-family: "AzureLinux"
spec:
  nodeClassRef:
    group: karpenter.azure.com
    kind: AKSNodeClass
    name: default
  requirements:
    - key: node.kubernetes.io/instance-type
      operator: In
      values: ["Standard_NC12s_v3"]
    - key: karpenter.sh/capacity-type
      operator: In
      values: ["on-demand"]
  resources:
    requests:
      storage: "120Gi"
  taints:
    - key: "sku"
      value: "gpu"
      effect: NoSchedule
```

## KAITO Workspace Examples

### Azure Linux Workspace with Annotation

```yaml
apiVersion: kaito.sh/v1beta1
kind: Workspace
metadata:
  name: falcon-7b-azure-linux
  annotations:
    kaito.sh/node-image-family: "AzureLinux"
spec:
  resource:
    instanceType: "Standard_NC24s_v3"
    count: 1
    labelSelector:
      matchLabels:
        workload: "falcon-7b"
  inference:
    preset:
      name: "falcon-7b"
```

## Implementation Details

The GPU Provisioner determines the OS SKU based on the following logic in `instance.go`:

1. **Check annotation on NodeClaim**:
   ```go
   if imageFamily, ok := nodeClaim.Annotations["kaito.sh/node-image-family"]; ok {
     // map image family to OSSKU
   }
   ```

2. **Default to Ubuntu** if annotation is not present

## Case Sensitivity

Image family values are **case-insensitive**. All of the following values will work:
- `AzureLinux`
- `azurelinux`
- `AZURELINUX`
- `AzUrElInUx`

## Benefits of Azure Linux

1. **Container-optimized**: Designed specifically for containerized workloads
2. **Security**: Enhanced security features and reduced attack surface
3. **Performance**: Optimized for cloud-native applications
4. **Microsoft Support**: Direct support from Microsoft
5. **Compliance**: Built with enterprise security and compliance in mind

## Validation

### Check Node OS Image

```bash
# Check the OS image on your nodes
kubectl get nodes -o custom-columns=NAME:.metadata.name,OS-IMAGE:.status.nodeInfo.osImage

# Example output for Azure Linux:
# NAME                                OS-IMAGE
# aks-azlinuxpool-12345678-vmss000000  Azure Linux 2.0.20240101
```

### Check Agent Pool OS SKU

```bash
# Check agent pool configuration
az aks agentpool show \
  --resource-group <resource-group> \
  --cluster-name <cluster-name> \
  --name <pool-name> \
  --query "osSkU"

# Expected output: "AzureLinux"
```

## Troubleshooting

### Common Issues

1. **Unknown image family warning**:
   ```
   Unknown imageFamily InvalidFamily in NodeClaim annotation, defaulting to Ubuntu
   ```
  **Solution**: Ensure the image family name is one of: `AzureLinux`, `Ubuntu`

2. **Case sensitivity confusion**:
   **Solution**: Remember that values are case-insensitive, so `azurelinux` works the same as `AzureLinux`

3. **Missing annotation**:
  **Solution**: Add `kaito.sh/node-image-family` in NodeClaim annotations when you need a non-default image family

### Debug Commands

```bash
# Check NodeClaim labels and annotations
kubectl get nodeclaim <nodeclaim-name> -o yaml

# Check GPU Provisioner logs
kubectl logs -n gpu-provisioner deployment/gpu-provisioner

# Check node labels and OS info
kubectl describe node <node-name>
```

## Migration Guide

### From Ubuntu to Azure Linux

1. **Update your NodeClaim or Workspace**:
   ```yaml
   # Add this annotation to your NodeClaim
   metadata:
     annotations:
       kaito.sh/node-image-family: "AzureLinux"
   ```

2. **Verify the change**:
   ```bash
   # Check that new nodes use Azure Linux
   kubectl get nodes -o custom-columns=NAME:.metadata.name,OS-IMAGE:.status.nodeInfo.osImage
   ```

3. **Test your workloads**:
   - Ensure your containerized workloads work correctly on Azure Linux
   - Most workloads should work without changes

## Best Practices

1. **Use NodeClaim annotations** to set image family
2. **Test thoroughly** when migrating existing workloads to Azure Linux
3. **Monitor resource usage** as Azure Linux may have different resource characteristics
4. **Keep GPU drivers updated** to ensure compatibility with Azure Linux
5. **Use specific instance types** that are known to work well with Azure Linux and your GPU workloads

## Related Links

- [Azure Linux Documentation](https://docs.microsoft.com/en-us/azure/azure-linux/)
- [KAITO Documentation](https://github.com/kaito-project/kaito)
- [GPU Provisioner Repository](https://github.com/Azure/gpu-provisioner)
