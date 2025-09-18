VERSION ?= v0.3.5
# Image URL to use all building/pushing image targets
REGISTRY ?= ghcr.io/azure/gpu-provisioner
IMG_NAME ?= gpu-provisioner
IMG_TAG ?= $(subst v,,$(VERSION))

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(abspath $(TOOLS_DIR)/bin)

GOLANGCI_LINT_VER := v1.61.0
GOLANGCI_LINT_BIN := golangci-lint
GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/$(GOLANGCI_LINT_BIN)-$(GOLANGCI_LINT_VER))

# Scripts
GO_INSTALL := ./hack/go-install.sh

# TEST_SUITE enables you to select a specific test suite directory to run "make e2etests" or "make test" against
TEST_SUITE ?= "..."
TEST_TIMEOUT ?= "1h"

REPO_ROOT ?= $(shell pwd)
LOCALBIN ?= $(REPO_ROOT)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

CONTROLLER_TOOLS_VERSION ?= v0.19.0
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

## --------------------------------------
## Tooling Binaries
## --------------------------------------

$(GOLANGCI_LINT):
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) github.com/golangci/golangci-lint/cmd/golangci-lint $(GOLANGCI_LINT_BIN) $(GOLANGCI_LINT_VER)

# build variables
BUILD_VERSION_VAR := $(REPO_PATH)/pkg/version.BuildVersion
BUILD_DATE_VAR := $(REPO_PATH)/pkg/version.BuildDate
BUILD_DATE := $$(date +%Y-%m-%d-%H:%M)
GIT_VAR := $(REPO_PATH)/pkg/version.GitCommit
GIT_HASH := $$(git rev-parse --short HEAD)
KARPENTER_VERSION_KEY := sigs.k8s.io/karpenter/pkg/operator.Version
KARPENTER_VERSION_VAL := $(shell git describe --tags --always | cut -d"v" -f2)
LDFLAGS ?= "-X $(BUILD_DATE_VAR)=$(BUILD_DATE) -X $(BUILD_VERSION_VAR)=$(IMAGE_VERSION) -X $(GIT_VAR)=$(GIT_HASH) -X $(KARPENTER_VERSION_KEY)=$(KARPENTER_VERSION_VAL)"

# AKS INT/Staging Test
AZURE_SUBSCRIPTION_ID ?= ff05f55d-22b5-44a7-b704-f9a8efd493ed
AZURE_LOCATION=eastus
AKS_K8S_VERSION ?= 1.30.5
AZURE_RESOURCE_GROUP ?= <YOUR RG>
AZURE_ACR_NAME ?= <YOUR ACR>
AZURE_CLUSTER_NAME ?= <YOUR CLUSTER>

az-login: ## Login into Azure
	az login
	az account set --subscription $(AZURE_SUBSCRIPTION_ID)

az-mkrg: ## Create resource group
	az group create --name $(AZURE_RESOURCE_GROUP) --location $(AZURE_LOCATION) -o none

az-mkacr: az-mkrg ## Create test ACR
	az acr create --name $(AZURE_ACR_NAME) --resource-group $(AZURE_RESOURCE_GROUP) --sku Standard --admin-enabled -o none
	az acr login  --name $(AZURE_ACR_NAME)

az-mkaks: az-mkacr ## Create test AKS cluster (with msi, oidc and workload identity enabled)
	az aks create  --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP) --attach-acr $(AZURE_ACR_NAME) \
	--kubernetes-version $(AKS_K8S_VERSION) --node-count 1 --generate-ssh-keys \
	--enable-managed-identity  --enable-workload-identity --enable-oidc-issuer --node-vm-size Standard_D2s_v3 -o none
	az aks get-credentials --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP)

az-rmrg: ## Destroy test ACR and AKS cluster by deleting the resource group (use with care!)
	az group delete --name $(AZURE_RESOURCE_GROUP)

.PHONE: az-identity-perm
az-identity-perm: ## Create identity for gpu-provisioner
	az identity create --name gpuIdentity --resource-group $(AZURE_RESOURCE_GROUP)
	$(eval IDENTITY_PRINCIPAL_ID=$(shell az identity show --name gpuIdentity --resource-group $(AZURE_RESOURCE_GROUP) --subscription $(AZURE_SUBSCRIPTION_ID) --query 'principalId'))
	$(eval IDENTITY_CLIENT_ID=$(shell az identity show --name gpuIdentity --resource-group $(AZURE_RESOURCE_GROUP) --subscription $(AZURE_SUBSCRIPTION_ID) --query 'clientId'))
	az role assignment create --assignee $(IDENTITY_PRINCIPAL_ID) --scope /subscriptions/$(AZURE_SUBSCRIPTION_ID)/resourceGroups/$(AZURE_RESOURCE_GROUP)/providers/Microsoft.ContainerService/managedClusters/$(AZURE_CLUSTER_NAME)  --role "Contributor"

.PHONY: az-patch-helm
az-patch-helm:  ## Update Azure client env vars and settings in helm values.yml
	az aks get-credentials --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP)
	$(eval IDENTITY_CLIENT_ID=$(shell az identity show --name gpuIdentity --resource-group $(AZURE_RESOURCE_GROUP) --query 'clientId' -o tsv))
	$(eval AZURE_TENANT_ID=$(shell az account show | jq -r ".tenantId"))

	yq -i '(.controller.image.repository)                                              = "$(REGISTRY)/gpu-provisioner"'                    ./charts/gpu-provisioner/values.yaml
	yq -i '(.controller.image.tag)                                                     = "$(IMG_TAG)"'                                     ./charts/gpu-provisioner/values.yaml
	yq -i '(.controller.env[] | select(.name=="LOCATION"))                      .value = "$(AZURE_LOCATION)"'                              ./charts/gpu-provisioner/values.yaml
	yq -i '(.controller.env[] | select(.name=="ARM_SUBSCRIPTION_ID"))           .value = "$(AZURE_SUBSCRIPTION_ID)"'                       ./charts/gpu-provisioner/values.yaml
	yq -i '(.controller.env[] | select(.name=="ARM_RESOURCE_GROUP"))            .value = "$(AZURE_RESOURCE_GROUP)"'                        ./charts/gpu-provisioner/values.yaml
	yq -i '(.controller.env[] | select(.name=="AZURE_CLUSTER_NAME"))            .value = "$(AZURE_CLUSTER_NAME)"'                          ./charts/gpu-provisioner/values.yaml
	yq -i '(.settings.azure.clusterName)                                               = "$(AZURE_CLUSTER_NAME)"'                          ./charts/gpu-provisioner/values.yaml
	yq -i '(.workloadIdentity.clientId)                                                = "$(IDENTITY_CLIENT_ID)"'                          ./charts/gpu-provisioner/values.yaml
	yq -i '(.workloadIdentity.tenantId)                                                = "$(AZURE_TENANT_ID)"'                             ./charts/gpu-provisioner/values.yaml

.PHONE: az-federated-credential
az-federated-credential:  ## Create federated credential for gpu-provisioner
	$(eval AKS_OIDC_ISSUER=$(shell az aks show -n "$(AZURE_CLUSTER_NAME)" -g "$(AZURE_RESOURCE_GROUP)" --subscription $(AZURE_SUBSCRIPTION_ID) --query "oidcIssuerProfile.issuerUrl"))
	az identity federated-credential create --name gpu-federatecredential --identity-name gpuIdentity --resource-group "$(AZURE_RESOURCE_GROUP)" --issuer "$(AKS_OIDC_ISSUER)" \
        --subject system:serviceaccount:"gpu-provisioner:gpu-provisioner" --audience api://AzureADTokenExchange --subscription $(AZURE_SUBSCRIPTION_ID)

#kubectl annotate sa gpu-provisioner -n gpu-provisioner azure.workload.identity/tenant-id="$(AZURE_TENANT_ID)" --overwrite

az-perm-acr:
	$(eval AZURE_CLIENT_ID=$(shell az aks show --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP) | jq  -r ".identityProfile.kubeletidentity.clientId"))
	$(eval AZURE_ACR_ID=$(shell az acr show --name $(AZURE_ACR_NAME)  --resource-group $(AZURE_RESOURCE_GROUP) | jq  -r ".id"))
	az role assignment create --assignee $(AZURE_CLIENT_ID) --scope $(AZURE_ACR_ID) --role "AcrPull"

az-build: ## Build the gpu-provisioner controller
	az acr login -n $(AZURE_ACR_NAME)

az-creds: ## Get cluster credentials
	az aks get-credentials --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP)

.PHONY: go-build
go-build:
	go build -a -ldflags $(LDFLAGS) -o _output/gpu-provisioner ./cmd/controller/main.go

##@ Docker
BUILDX_BUILDER_NAME ?= img-builder
OUTPUT_TYPE ?= type=registry
QEMU_VERSION ?= 5.2.0-2
ARCH ?= amd64,arm64
BUILDKIT_VERSION ?= v0.18.1

.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	@if ! docker buildx ls | grep $(BUILDX_BUILDER_NAME); then \
		docker run --rm --privileged multiarch/qemu-user-static:$(QEMU_VERSION) --reset -p yes; \
		docker buildx create --name $(BUILDX_BUILDER_NAME) --driver-opt image=mcr.microsoft.com/oss/v2/moby/buildkit:$(BUILDKIT_VERSION) --use; \
		docker buildx inspect $(BUILDX_BUILDER_NAME) --bootstrap; \
	fi

.PHONY: docker-build
docker-build: docker-buildx
	docker buildx build \
		--file ./Dockerfile \
		--output=$(OUTPUT_TYPE) \
		--platform="linux/$(ARCH)" \
		--pull \
		--build-arg="KARPENTERVER=$(KARPENTER_VERSION_VAL)" \
		--tag $(REGISTRY)/$(IMG_NAME):$(IMG_TAG) .


## --------------------------------------
## Linting
## --------------------------------------
.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run -v

## --------------------------------------
## Tests
## --------------------------------------

.PHONY: unit-test
unit-test: ## Run unit tests.
	go test -v ./pkg/providers/instance ./pkg/cloudprovider  \
	-race -coverprofile=coverage.txt -covermode=atomic fmt
	go tool cover -func=coverage.txt

.PHONY: e2etests
e2etests: ## Run the e2e suite against your local cluster
	cd test && CLUSTER_NAME=${CLUSTER_NAME} go test \
		-p 1 \
		-count 1 \
		-timeout ${TEST_TIMEOUT} \
		-v \
		./e2e/suites/suite_test.go \
		--ginkgo.timeout=${TEST_TIMEOUT} \
		--ginkgo.grace-period=3m \
		--ginkgo.vv

## --------------------------------------
## Release
## To create a release, run `make release VERSION=x.y.z`
## --------------------------------------
.PHONY: release-manifest
release-manifest:
	@sed -i -e 's/^VERSION ?= .*/VERSION ?= ${VERSION}/' ./Makefile
	@sed -i -e "s/version: .*/version: ${IMG_TAG}/" ./charts/gpu-provisioner/Chart.yaml
	@sed -i -e "s/appVersion: .*/appVersion: ${IMG_TAG}/" ./charts/gpu-provisioner/Chart.yaml
	@sed -i -e "s/tag: .*/tag: ${IMG_TAG}/" ./charts/gpu-provisioner/values.yaml
	@sed -i -e 's/gpu-provisioner: .*/gpu-provisioner:${IMG_TAG}/' ./charts/gpu-provisioner/README.md
	@sed -i -e 's/CHART_VERSION=.*/CHART_VERSION=${IMG_TAG}/' ./charts/gpu-provisioner/README.md
	git checkout -b release-${VERSION}
	git add ./Makefile ./charts/gpu-provisioner/Chart.yaml ./charts/gpu-provisioner/values.yaml ./charts/gpu-provisioner/README.md
	git commit -s -m "release: update manifest and helm charts for ${VERSION}"

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(CONTROLLER_GEN) && $(CONTROLLER_GEN) --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: controller-gen ## Generate CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd paths="./..." output:crd:artifacts:config=charts/gpu-provisioner/crds

