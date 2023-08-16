#!/usr/bin/env bash
set -euo pipefail

if [ -z ${ENABLE_GIT_PUSH+x} ];then
  ENABLE_GIT_PUSH=false
fi

echo "api-code-gen running ENABLE_GIT_PUSH: ${ENABLE_GIT_PUSH}"

pricing() {
  GENERATED_FILE="pkg/providers/pricing/zz_generated.pricing.go"
  NO_UPDATE=$' pkg/providers/pricing/zz_generated.pricing.go | 4 ++--\n 1 file changed, 2 insertions(+), 2 deletions(-)'
  SUBJECT="Pricing"

  go run hack/code/prices/prices_gen.go -- "${GENERATED_FILE}"

  GIT_DIFF=$(git diff --stat "${GENERATED_FILE}")
  checkForUpdates "${GIT_DIFF}" "${NO_UPDATE}" "${SUBJECT} beside timestamps since last update" "${GENERATED_FILE}"
}

skugen() {
  # Note to use skugen you need valid azure credentials, and references to token credentials 
  # you can create a token credential by running the following command: 
  # az ad sp create-for-rbac --name "skugen" --role contributor --scopes /subscriptions/<subscription-id>/resourceGroups/<resource-group> 
  # and then export the following environment variables that use the returned Service Principal JSON: 
  # export TENANT_ID=<tenant>
  # export AAD_CLIENT_ID=<appId>
  # export AAD_CLIENT_SECRET=<password>
  GENERATED_FILE=$(PWD)/"pkg/fake/zz_generated.sku.go"
  echo GENERATED_FILE: "${GENERATED_FILE}"
  SUBJECT="SKUGEN"
  NO_UPDATE=$' pkg/fake/zz_generated.sku.go | 2 +- 1 file changed, 1 insertion(+), 1 deletion(-)' 
 # File, region, and skus   
  go run hack/code/instancetype_testdata_gen.go -- "${GENERATED_FILE}" "" "Standard_D2_v2,Standard_D2_v3,Standard_DS2_v2,Standard_D2s_v3,Standard_D2_v5,Standard_F16s_v2" 


  GIT_DIFF=$(git diff --stat "${GENERATED_FILE}")
  checkForUpdates "${GIT_DIFF}" "${NO_UPDATE}" "${SUBJECT} beside timestamps since last update" "${GENERATED_FILE}"
}

checkForUpdates() {
  GIT_DIFF=$1
  NO_UPDATE=$2
  SUBJECT=$3
  GENERATED_FILE=$4

  echo "Checking git diff for updates. ${GIT_DIFF}, ${NO_UPDATE}"
  if [[ "${GIT_DIFF}" == "${NO_UPDATE}" ]]; then
    noUpdates "${SUBJECT}"
    git checkout "${GENERATED_FILE}"
  else
    echo "true" >/tmp/api-code-gen-updates
    git add "${GENERATED_FILE}"
    if [[ $ENABLE_GIT_PUSH == true ]]; then
      gitCommitAndPush "${SUBJECT}"
    fi
  fi
}

gitOpenAndPullBranch() {
  git fetch origin
  git checkout api-code-gen || git checkout -b api-code-gen || true
}

gitCommitAndPush() {
  UPDATE_SUBJECT=$1
  git commit -m "APICodeGen updates from Azure API for ${UPDATE_SUBJECT}"
  git push --set-upstream origin api-code-gen
}

noUpdates() {
  UPDATE_SUBJECT=$1
  echo "No updates from Azure API for ${UPDATE_SUBJECT}"
}

if [[ $ENABLE_GIT_PUSH == true ]]; then
  gitOpenAndPullBranch
fi

# Run all the codegen scripts
pricing
skugen
