# Image URL to use all building/pushing image targets
IMG_NAME ?= gpu-provisioner
IMG_TAG ?= 0.1.0

# build variables
BUILD_VERSION_VAR := $(REPO_PATH)/pkg/version.BuildVersion
BUILD_DATE_VAR := $(REPO_PATH)/pkg/version.BuildDate
BUILD_DATE := $$(date +%Y-%m-%d-%H:%M)
GIT_VAR := $(REPO_PATH)/pkg/version.GitCommit
GIT_HASH := $$(git rev-parse --short HEAD)
LDFLAGS ?= "-X $(BUILD_DATE_VAR)=$(BUILD_DATE) -X $(BUILD_VERSION_VAR)=$(IMAGE_VERSION) -X $(GIT_VAR)=$(GIT_HASH)"

# Azure Container Service - Test
# AZURE_SUBSCRIPTION_ID=8ecadfc9-d1a3-4ea4-b844-0d9f87e4d7c8
# Data Plane Developer
# AZURE_SUBSCRIPTION_ID=8643025a-c059-4a48-85d0-d76f51d63a74
# AKS INT/Staging Test
AZURE_SUBSCRIPTION_ID=ff05f55d-22b5-44a7-b704-f9a8efd493ed
# Note: for experimental SAVM support, location has to be westus2
AZURE_LOCATION=eastus
ifeq ($(CODESPACES),true)
  AZURE_RESOURCE_GROUP=$(CODESPACE_NAME)
  AZURE_ACR_NAME=$(subst -,,$(CODESPACE_NAME))
else
  AZURE_RESOURCE_GROUP=llm-test
  AZURE_ACR_NAME=aimodelsregistry
endif

AZURE_CLUSTER_NAME=new_demo
AZURE_RESOURCE_GROUP_MC=MC_$(AZURE_RESOURCE_GROUP)_$(AZURE_CLUSTER_NAME)_$(AZURE_LOCATION)

az-all:      az-login az-mkaks      az-perm az-patch-skaffold-kubenet az-build az-run az-run-sample ## Provision the infra (ACR,AKS); build and deploy Karpenter; deploy sample Provisioner and workload
az-all-savm: az-login az-mkaks-savm az-perm az-patch-skaffold-azure   az-build az-run az-run-sample ## Provision the infra (ACR,AKS); build and deploy Karpenter; deploy sample Provisioner and workload - StandaloneVirtualMachines

az-login: ## Login into Azure
	az login
	az account set --subscription $(AZURE_SUBSCRIPTION_ID)

az-mkrg: ## Create resource group
	az group create --name $(AZURE_RESOURCE_GROUP) --location $(AZURE_LOCATION) -o none

az-mkacr: az-mkrg ## Create test ACR
	az acr create --name $(AZURE_ACR_NAME) --resource-group $(AZURE_RESOURCE_GROUP) --sku Standard --admin-enabled -o none
	az acr login  --name $(AZURE_ACR_NAME)
	skaffold config set default-repo $(AZURE_ACR_NAME).azurecr.io/gpu-ap

az-mkaks: az-mkacr ## Create test AKS cluster (with --vm-set-type AvailabilitySet for compatibility with standalone VMs)
	az aks create          --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP) --attach-acr $(AZURE_ACR_NAME) \
		--enable-managed-identity --node-count 1 --generate-ssh-keys --vm-set-type VirtualMachineScaleSets -o none
	az aks get-credentials --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP)

az-mkaks-savm: az-mkrg ## Create experimental cluster with standalone VMs (+ ACR)
	az deployment group create --resource-group $(AZURE_RESOURCE_GROUP) --template-file hack/azure/aks-savm.bicep --parameters aksname=$(AZURE_CLUSTER_NAME) acrname=$(AZURE_ACR_NAME)
	az aks get-credentials     --resource-group $(AZURE_RESOURCE_GROUP) --name $(AZURE_CLUSTER_NAME)
	skaffold config set default-repo $(AZURE_ACR_NAME).azurecr.io/gpu-ap


az-rmrg: ## Destroy test ACR and AKS cluster by deleting the resource group (use with care!)
	az group delete --name $(AZURE_RESOURCE_GROUP)

az-patch-skaffold: 	## Update Azure client env vars and settings in skaffold config
	$(eval AZURE_CLIENT_ID=$(shell az aks show --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP) | jq -r ".identityProfile.kubeletidentity.clientId"))
	$(eval CLUSTER_ENDPOINT=$(shell kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}'))
	# bootstrap token
	$(eval TOKEN_SECRET_NAME=$(shell kubectl get -n kube-system secrets --field-selector=type=bootstrap.kubernetes.io/token -o jsonpath='{.items[0].metadata.name}'))
	$(eval TOKEN_ID=$(shell          kubectl get -n kube-system secret $(TOKEN_SECRET_NAME) -o jsonpath='{.data.token-id}'     | base64 -d))
	$(eval TOKEN_SECRET=$(shell      kubectl get -n kube-system secret $(TOKEN_SECRET_NAME) -o jsonpath='{.data.token-secret}' | base64 -d))
	$(eval BOOTSTRAP_TOKEN=$(TOKEN_ID).$(TOKEN_SECRET))
	# ssh key 
	$(eval SSH_PUBLIC_KEY=$(shell cat ~/.ssh/id_rsa.pub) azureuser)
	yq -i '(.manifests.helm.releases[0].overrides.controller.env[] | select(.name=="ARM_SUBSCRIPTION_ID"))           .value = "$(AZURE_SUBSCRIPTION_ID)"'   skaffold.yaml
	yq -i '(.manifests.helm.releases[0].overrides.controller.env[] | select(.name=="LOCATION"))                      .value = "$(AZURE_LOCATION)"'          skaffold.yaml
	yq -i '(.manifests.helm.releases[0].overrides.controller.env[] | select(.name=="ARM_USER_ASSIGNED_IDENTITY_ID")) .value = "$(AZURE_CLIENT_ID)"'         skaffold.yaml
	yq -i '(.manifests.helm.releases[0].overrides.controller.env[] | select(.name=="ARM_RESOURCE_GROUP"))            .value = "$(AZURE_RESOURCE_GROUP)"'    skaffold.yaml
	yq -i '(.manifests.helm.releases[0].overrides.controller.env[] | select(.name=="AZURE_NODE_RESOURCE_GROUP"))     .value = "$(AZURE_RESOURCE_GROUP_MC)"' skaffold.yaml
	yq -i '(.manifests.helm.releases[0].overrides.controller.env[] | select(.name=="AZURE_CLUSTER_NAME"))            .value = "$(AZURE_CLUSTER_NAME)"'      skaffold.yaml
	yq -i  '.manifests.helm.releases[0].overrides.settings.azure.clusterName =                                                "$(AZURE_CLUSTER_NAME)"'      skaffold.yaml
	yq -i  '.manifests.helm.releases[0].overrides.settings.azure.clusterEndpoint =                                            "$(CLUSTER_ENDPOINT)"'        skaffold.yaml
	yq -i  '.manifests.helm.releases[0].overrides.settings.azure.networkPlugin =                                              "azure"'                      skaffold.yaml
	yq -i  '.manifests.helm.releases[0].overrides.settings.azure.kubeletClientTLSBootstrapToken =                             "$(BOOTSTRAP_TOKEN)"'         skaffold.yaml
	yq -i  '.manifests.helm.releases[0].overrides.settings.azure.sshPublicKey =                                               "$(SSH_PUBLIC_KEY)"'          skaffold.yaml

	skaffold config set default-repo $(AZURE_ACR_NAME).azurecr.io/gpu-ap

az-patch-skaffold-kubenet: az-patch-skaffold
	$(eval AZURE_SUBNET_ID=$(shell az network vnet list --resource-group $(AZURE_RESOURCE_GROUP_MC) | jq  -r ".[0].subnets[0].id"))
	yq -i '(.manifests.helm.releases[0].overrides.controller.env[] | select(.name=="AZURE_SUBNET_ID"))               .value = "$(AZURE_SUBNET_ID)"'         skaffold.yaml
	yq -i  '.manifests.helm.releases[0].overrides.settings.azure.networkPlugin =                                              "kubenet"'                    skaffold.yaml

az-patch-skaffold-azure: az-patch-skaffold
	$(eval AZURE_SUBNET_ID=$(shell az aks show --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP) | jq -r ".agentPoolProfiles[0].vnetSubnetId"))
	yq -i '(.manifests.helm.releases[0].overrides.controller.env[] | select(.name=="AZURE_SUBNET_ID"))               .value = "$(AZURE_SUBNET_ID)"'         skaffold.yaml

az-mkvmssflex: ## Create VMSS Flex (optional, only if creating VMs referencing this VMSS)
	az vmss create --name $(AZURE_CLUSTER_NAME)-vmss --resource-group $(AZURE_RESOURCE_GROUP_MC) --location $(AZURE_LOCATION) \
		--instance-count 0 --orchestration-mode Flexible --platform-fault-domain-count 1 --zones 1 2 3

az-rmvmss-vms: ## Delete all VMs in VMSS Flex (use with care!)
	az vmss delete-instances --name $(AZURE_CLUSTER_NAME)-vmss --resource-group $(AZURE_RESOURCE_GROUP_MC) --instance-ids '*'

az-perm: ## Create role assignments to let Karpenter manage VMs and Network
	$(eval AZURE_CLIENT_ID=$(shell az aks show --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP) | jq  -r ".identityProfile.kubeletidentity.clientId"))
	az role assignment create --assignee $(AZURE_CLIENT_ID) --scope /subscriptions/$(AZURE_SUBSCRIPTION_ID)/resourceGroups/$(AZURE_RESOURCE_GROUP_MC) --role "Virtual Machine Contributor"
	az role assignment create --assignee $(AZURE_CLIENT_ID) --scope /subscriptions/$(AZURE_SUBSCRIPTION_ID)/resourceGroups/$(AZURE_RESOURCE_GROUP)    --role "Contributor"
	az role assignment create --assignee $(AZURE_CLIENT_ID) --scope /subscriptions/$(AZURE_SUBSCRIPTION_ID)/resourceGroups/$(AZURE_RESOURCE_GROUP)    --role "Azure Kubernetes Service RBAC Cluster Admin"
	@echo Consider "make az-patch-skaffold"!

az-perm-acr:
	$(eval AZURE_CLIENT_ID=$(shell az aks show --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP) | jq  -r ".identityProfile.kubeletidentity.clientId"))
	$(eval AZURE_ACR_ID=$(shell    az acr show --name $(AZURE_ACR_NAME)     --resource-group $(AZURE_RESOURCE_GROUP) | jq  -r ".id"))
	az role assignment create --assignee $(AZURE_CLIENT_ID) --scope $(AZURE_ACR_ID) --role "AcrPull"

az-build: ## Build the gpu-provisioner controller and webhook images using skaffold build (which uses ko build)
	az acr login -n $(AZURE_ACR_NAME)
	skaffold build

az-creds: ## Get cluster credentials
	az aks get-credentials --name $(AZURE_CLUSTER_NAME) --resource-group $(AZURE_RESOURCE_GROUP)

az-run: ## Deploy the controller from the current state of your git repository into your ~/.kube/config cluster using skaffold run
	az acr login -n $(AZURE_ACR_NAME)
	skaffold run

az-run-sample: ## Deploy sample Provisioner and workload (with 0 replicas, to be scaled manually)
	kubectl apply -f examples/provisioner/general-purpose-azure.yaml 
	kubectl apply -f examples/workloads/inflate.yaml 

az-dev: ## Deploy and develop using skaffold dev
	skaffold dev

az-debug: ## Rebuild, deploy and debug using skaffold debug
	az acr login -n $(AZURE_ACR_NAME)
	skaffold delete || true
	skaffold debug # --platform=linux/arm64

az-cleanup: ## Delete the deployment
	skaffold delete || true

az-mkgohelper: ## Build and configure custom go-helper-image for skaffold
	cd hack/go-helper-image; docker build . --tag $(AZURE_ACR_NAME).azurecr.io/skaffold-debug-support/go # --platform=linux/arm64
	az acr login -n $(AZURE_ACR_NAME)
	docker push $(AZURE_ACR_NAME).azurecr.io/skaffold-debug-support/go
	skaffold config set --global debug-helpers-registry $(AZURE_ACR_NAME).azurecr.io/skaffold-debug-support	

az-rmnodes-fin: ## Remove finalizer from all nodes (use with care!)
	for node in $$(kubectl get nodes -l karpenter.sh/provisioner-name --output=jsonpath={.items..metadata.name}); do \
		kubectl patch node $$node -p '{"metadata":{"finalizers":null}}'; \
	done	

az-rmnodes: ## kubectl delete all Karpenter-provisioned nodes; don't wait for finalizers (use with care!)
	kubectl delete --wait=false nodes -l karpenter.sh/provisioner-name
    # kubectl wait --for=delete nodes -l karpenter.sh/provisioner-name --timeout=10m

az-rmmachines-fin: ## Remove Karpenter finalizer from all machines (use with care!)
	for machine in $$(kubectl get machines --output=jsonpath={.items..metadata.name}); do \
		kubectl patch machine $$machine --type=json -p '[{"op": "remove", "path": "/metadata/finalizers"}]'; \
	done	

az-rmmachines: ## kubectl delete all Machines; don't wait for finalizers (use with care!)
	kubectl delete --wait=false machines --all

az-perftest1: ## Test scaling out/in (1 VM)
	hack/azure/perftest.sh 1

az-perftest5: ## Test scaling out/in (5 VMs)
	hack/azure/perftest.sh 5

az-perftest20: ## Test scaling out/in (20 VMs)
	hack/azure/perftest.sh 20

az-perftest100: ## Test scaling out/in (100 VMs)
	hack/azure/perftest.sh 100

az-perftest300: ## Test scaling out/in (300 VMs)
	hack/azure/perftest.sh 300

az-perftest400: ## Test scaling out/in (400 VMs)
	hack/azure/perftest.sh 400

az-perftest500: ## Test scaling out/in (500 VMs)
	hack/azure/perftest.sh 500

az-perftest1000: ## Test scaling out/in (1000 VMs)
	hack/azure/perftest.sh 1000

az-resg: ## List resources in MC rg
	az resource list -o table -g $(AZURE_RESOURCE_GROUP_MC)

az-res: ## List resource created by Karpenter (assume 'default' Provisioner)
	az resource list -o table --tag=karpenter.sh_provisioner-name=default

az-resc: ## Count resource created by Karpenter (assume 'default' Provisioner)
	az resource list -o table --tag=karpenter.sh_provisioner-name=default | tail -n +3 | wc -l

az-rmres: ## Delete (az resource delete) all resources created by Karpenter (assume 'default' Provisioner). Use with extra care!
	az resource list --tag=karpenter.sh_provisioner-name=default --query "[].[id]" -o tsv | xargs --verbose -n 5 az resource delete --ids

az-rmres4: ## Delete (az resource delete) all resources created by Karpenter (d[s]v[23] provisioners). Use with extra care!
	az resource list --tag=karpenter.sh_provisioner-name=dv2  --query "[].[id]" -o tsv | xargs --verbose -n 10 az resource delete --ids || true
	az resource list --tag=karpenter.sh_provisioner-name=dv3  --query "[].[id]" -o tsv | xargs --verbose -n 10 az resource delete --ids || true
	az resource list --tag=karpenter.sh_provisioner-name=dsv2 --query "[].[id]" -o tsv | xargs --verbose -n 10 az resource delete --ids || true
	az resource list --tag=karpenter.sh_provisioner-name=dsv3 --query "[].[id]" -o tsv | xargs --verbose -n 10 az resource delete --ids || true

az-portal: ## Get Azure Portal links for relevant resource groups
	@echo https://ms.portal.azure.com/#@microsoft.onmicrosoft.com/asset/HubsExtension/ResourceGroups/subscriptions/$(AZURE_SUBSCRIPTION_ID)/resourceGroups/$(AZURE_RESOURCE_GROUP)
	@echo https://ms.portal.azure.com/#@microsoft.onmicrosoft.com/asset/HubsExtension/ResourceGroups/subscriptions/$(AZURE_SUBSCRIPTION_ID)/resourceGroups/$(AZURE_RESOURCE_GROUP_MC)

az-list-skus: ## List all public VM images from microsoft-aks
	az vm image list-skus --publisher microsoft-aks --location $(AZURE_LOCATION) --offer aks -o table

az-list-usage: ## List VM usage/quotas
	az vm list-usage --location $(AZURE_LOCATION) -o table | grep "Family vCPU"

az-ratelimits: ## Show remaining ARM requests for subscription
	@az group create --name $(AZURE_RESOURCE_GROUP) --location $(AZURE_LOCATION) --debug 2>&1 | grep x-ms-ratelimit-remaining-subscription-writes
	@az group show   --name $(AZURE_RESOURCE_GROUP)                              --debug 2>&1 | grep x-ms-ratelimit-remaining-subscription-reads

az-kdebug: ## Inject ephemeral debug container (kubectl debug) into Karpenter pod
	$(eval POD=$(shell kubectl get pods -l app.kubernetes.io/name=karpenter -n karpenter -o name))
	kubectl debug -n karpenter $(POD) --image wbitt/network-multitool -it -- sh

az-klogs: ## Karpenter logs
	$(eval POD=$(shell kubectl get pods -l app.kubernetes.io/name=karpenter -n karpenter -o name))
	kubectl logs -f -n karpenter $(POD)

az-kevents: ## Karpenter events
	kubectl get events -A --field-selector source=karpenter

az-node-viewer: ## Watch nodes using eks-node-viewer
	eks-node-viewer --disable-pricing --node-selector "karpenter.sh/provisioner-name" # --resources cpu,memory

.PHONY: go-build
go-build:
	go build -a -ldflags $(LDFLAGS) -o _output/gpu-provisioner ./cmd/controller/main.go

##@ Docker
BUILDX_BUILDER_NAME ?= img-builder
OUTPUT_TYPE ?= type=registry
QEMU_VERSION ?= 5.2.0-2
ARCH ?= amd64

.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	@if ! docker buildx ls | grep $(BUILDX_BUILDER_NAME); then \
		docker run --rm --privileged multiarch/qemu-user-static:$(QEMU_VERSION) --reset -p yes; \
		docker buildx create --name $(BUILDX_BUILDER_NAME) --use; \
		docker buildx inspect $(BUILDX_BUILDER_NAME) --bootstrap; \
	fi

.PHONY: docker-build
docker-build: docker-buildx
	az acr login  --name $(AZURE_ACR_NAME)
	docker buildx build \
		--file ./Dockerfile \
		--output=$(OUTPUT_TYPE) \
		--platform="linux/$(ARCH)" \
		--pull \
		--tag $(AZURE_ACR_NAME).azurecr.io/$(IMG_NAME):$(IMG_TAG) .
