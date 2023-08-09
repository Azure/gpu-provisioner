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
