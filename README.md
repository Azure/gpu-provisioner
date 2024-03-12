# Azure GPU Provisioner
[![Go Report Card](https://goreportcard.com/badge/github.com/Azure/gpu-provisioner)](https://goreportcard.com/report/github.com/Azure/gpu-provisioner)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/Azure/gpu-provisioner)
[![codecov](https://codecov.io/gh/Azure/gpu-provisioner/graph/badge.svg?token=b7B1G5dtk1)](https://codecov.io/gh/Azure/gpu-provisioner)

gpu-Provisioner is an [Azure Karpenter provider](https://github.com/Azure/karpenter-provider-azure) implementation for [Karpenter](https://karpenter.sh/) machine API. It leverage the `machine` CRD introduced by [Karpenter](https://karpenter.sh/) to orchestrate the GPU VM provisioning and its lifecycle management in a standard AKS cluster.
It implements the cloud provider interfaces to realize the following abstraction:
`machine` -> `AKS agent pool` (with vmss and a hard limit of VM count to 1)

```
VERSION=v0.1.0 make docker-build
make az-identity-perm
make az-patch-helm
helm install gpu-provisioner /charts/gpu-provisioner
make az-federated-credential

```
You should have a running controller in `gpu-provisioner` namespace.

## How to test
After deploying the controller successfully, one can apply the yaml in `/example` to create a machine CR. A real node will be created and added to the cluster by the controller.

## Important note
- The gpu-provisioner assumes the Machine CR name is **equal** to the agent pool name. Hence, **the machine CR name must be 1-11 characters in length, start with a letter, and the only allowed characters are letters and numbers**.
- The machine CR needs to have a label with key `kaito.sh/workspace`.

## Source Attribution

Notice: Files in this source code originated from a fork of https://github.com/aws/karpenter
which is under an Apache 2.0 license. Those files have been modified to reflect environmental requirements in AKS and Azure.

Many thanks to @ellistarn, @jonathan-innis, @tzneal, @bwagner5, @njtran, and many other developers active in the Karpenter community for laying the foundations of a Karpenter provider ecosystem!

Many thanks to @Bryce-Soghigian, @rakechill, @charliedmcb, @jackfrancis, @comtalyst, @aagusuab, @matthchr, @gandhipr, @dtzar for contributing to AKS Karpenter Provider!

## Contributing

[Read more](CONTRIBUTING.md)
<!-- markdown-link-check-disable -->
This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit <https://cla.opensource.microsoft.com>.

When you submit a pull request, a CLA bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., status check, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

## Trademarks
This project may contain trademarks or logos for projects, products, or services. Authorized use of Microsoft
trademarks or logos is subject to and must follow [Microsoft's Trademark & Brand Guidelines](https://www.microsoft.com/legal/intellectualproperty/trademarks/usage/general).
Use of Microsoft trademarks or logos in modified versions of this project must not cause confusion or imply Microsoft sponsorship.
Any use of third-party trademarks or logos are subject to those third-party's policies.

## Code of Conduct

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/). For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

<!-- markdown-link-check-enable -->
## Contact

"Kaito devs" <kaito@microsoft.com>
