# Azure GPU VM Provisioner


This is a fork of Karpenter machine controller.



## How to clone

### Rename the module

```
for each in $(find pkg/ -type f -follow -print); do sed "s/github.com\/Azure\/karpenter/github.com\/gpu-vmprovisioner/g" -i $each;done;
for each in $(find cmd/ -type f -follow -print); do sed "s/github.com\/Azure\/karpenter/github.com\/gpu-v
mprovisioner/g" -i $each;done;
```
### Edit go.mod

Remove `github.com/Azure/karpenter`.


### Vendor all modules

Change vendor code to disable controllers from karpenter-core package.



## How to run

For now, using `Makefile-az.mk` primarily. The `skaffold` tool is used for CI/CD. `skaffold.yaml` contains everything for building image and customizing the helm chart.


`make az-all` is for all. But steps like `az-mkaks`, `az-perm`, `az-patch-skaffold-kubenet` are the most important ones. Make sure to setup `AZURE_RESOURCE_GROUP` correctly in `Makefile-az.mk`.


Once deploying the controller successfully, apply all the yamls in `/example`. Creating the machine CR should lead to a new node added to the cluster.
