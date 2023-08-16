# Azure GPU VM Provisioner
This is a fork of the `Karpenter` machine controller. 

## How to clone
After cloning the repo from `https://github.com/Azure/karpenter`,
### Rename the module
```
for each in $(find pkg/ -type f -follow -print); do sed "s/github.com\/Azure\/karpenter/github.com\/gpu-vmprovisioner/g" -i $each;done;
for each in $(find cmd/ -type f -follow -print); do sed "s/github.com\/Azure\/karpenter/github.com\/gpu-vmprovisioner/g" -i $each;done;
```
### Edit the `go.mod`
Remove `github.com/Azure/karpenter` and change the module name to `github.com/gpu-vmprovisioner`.

### Vendor all modules
Change vendor code to disable controllers from the karpenter-core package.

## How to build

For now, all required steps are mentioned in `Makefile-az.mk`. The `skaffold` tool is used for CI/CD. `skaffold.yaml` contains everything for building the image and customizing the helm chart.

A one-for-all command is `make az-all`. You can run individual steps like `az-mkaks`, `az-perm`, `az-patch-skaffold-kubenet` which are the most important ones. Make sure to setup `AZURE_RESOURCE_GROUP`, `AZURE_ACR_NAME`, and `AZURE_CLUSTER_NAME` correctly in `Makefile-az.mk`.

If you already have a working cluster, just run 
```
make az-perm
make az-patch-skaffold-kubenet
make az-run
```
You should have a running controller in `gpu-provisioner` namespace.

## How to test
After deploying the controller successfully, one can apply the yaml in `/example` to create a machine CR. A real node will be created and added to the cluster by the controller.
