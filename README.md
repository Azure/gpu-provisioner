# Azure GPU VM Provisioner
This is a fork of the `Karpenter` machine controller. It leverage the `machine` CRD introduced by `Karpenter` to orchestrate the GPU VM provisioning and its lifecycle management in a standard AKS cluster.
It implements the cloud provider interfaces to realize the following abstraction:
`machine` -> `AKS agent pool` (with vmss and a hard limit of VM count to 1)

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

For now, all required steps are mentioned in `Makefile-az.mk`.

```
VERSION=v0.1.0 make docker-build
make az-perm
make az-patch-helm
helm install gpu-provisioner /charts/gpu-provisioner

```
You should have a running controller in `gpu-provisioner` namespace.

## How to test
After deploying the controller successfully, one can apply the yaml in `/example` to create a machine CR. A real node will be created and added to the cluster by the controller.
