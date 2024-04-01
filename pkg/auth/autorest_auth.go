/*
       Copyright (c) Microsoft Corporation.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package auth

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/azure/gpu-provisioner/pkg/utils"
	armopts "github.com/azure/gpu-provisioner/pkg/utils/opts"
	"github.com/pkg/errors"
)

const (
	E2ERPRefererEndpoint = "rp.e2e.ig.e2e-aks.azure.com"
)

// authResult contains the subset of results from token acquisition operation in ConfidentialClientApplication
// For details see https://aka.ms/msal-net-authenticationresult
type authResult struct {
	accessToken    string
	expiresOn      time.Time
	grantedScopes  []string
	declinedScopes []string
}

func NewAuthorizer(config *Config, env *azure.Environment) (autorest.Authorizer, error) {
	endpoint := env.ResourceManagerEndpoint
	scope := []string{strings.TrimSuffix(endpoint, "/") + "/.default"}
	ctx := context.Background()

	isE2E := utils.WithDefaultBool("E2E_TEST_MODE", false)
	if isE2E {
		endpoint = "https://" + E2ERPRefererEndpoint
		scope = []string{strings.TrimSuffix(endpoint, "/") + "/.default"}
		return getTokenForE2E(ctx, config, scope)
	}

	// Azure AD Workload Identity webhook will inject the following env vars:
	// 	AZURE_FEDERATED_TOKEN_FILE is the service account token path
	// 	AZURE_AUTHORITY_HOST is the AAD authority hostname
	fmt.Print("inside NewAuthorizer")
	tokenFilePath := os.Getenv("AZURE_FEDERATED_TOKEN_FILE")
	if tokenFilePath == "" {
		return nil, fmt.Errorf("required environment variable is not set, AZURE_FEDERATED_TOKEN_FILE: %s", tokenFilePath)
	}
	confidentialClientApp, err := GetConfidentialCertificate(config, &ClientAssertionCredential{file: tokenFilePath})
	if err != nil {
		fmt.Printf("error	from GetConfidentialCertificate: %s", err)
		return nil, err
	}
	result, err := confidentialClientApp.AcquireTokenSilent(ctx, scope)
	if err != nil {
		result, err = confidentialClientApp.AcquireTokenByCredential(ctx,
			scope)
		if err != nil {
			fmt.Printf("error from confidentialClientApp.AcquireTokenByCredential: %s", err)
			return autorest.NewBearerAuthorizer(authResult{}), errors.Wrap(err, "failed to acquire token")
		}
		fmt.Println("Access Token Is " + result.AccessToken)
	}
	fmt.Println("Silently acquired token " + result.AccessToken)
	return autorest.NewBearerAuthorizer(authResult{
		accessToken:    result.AccessToken,
		expiresOn:      result.ExpiresOn,
		grantedScopes:  result.GrantedScopes,
		declinedScopes: result.DeclinedScopes,
	}), nil
}

func getTokenForE2E(ctx context.Context, config *Config, scope []string) (autorest.Authorizer, error) {
	clientID := os.Getenv("AZURE_CLIENT_ID")
	if clientID == "" {
		return nil, fmt.Errorf("required environment variable is not set, AZURE_CLIENT_ID: %s", clientID)

	}
	armClientCert, err := getE2ETestingCert()
	if err != nil {
		return nil, err
	}

	cert, privateKey, err := azidentity.ParseCertificates([]byte(armClientCert), nil)
	if err != nil {
		fmt.Println(" confidential.CertFromPEM: ", err)
		return nil, err
	}

	clientCertificateCredential, err := azidentity.NewClientCertificateCredential(config.TenantID, clientID, cert, privateKey, &azidentity.ClientCertificateCredentialOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzurePublic,
			Retry: armopts.DefaultRetryOpts(),
		},
	})
	if err != nil {
		fmt.Printf("error	from NewClientCertificateCredential: %s", err)
		return nil, err
	}

	token, err := clientCertificateCredential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: scope,
	})
	if err != nil {
		fmt.Printf("error	from  client.GetToken: %s", err)
		return nil, err
	}
	return autorest.NewBearerAuthorizer(authResult{
		accessToken: token.Token,
		expiresOn:   token.ExpiresOn,
	}), nil
}

// OAuthToken implements the OAuthTokenProvider interface.  It returns the current access token.
func (ar authResult) OAuthToken() string {
	return ar.accessToken
}

func (a *authResult) WithAuthorization() autorest.PrepareDecorator {
	return autorest.WithBearerAuthorization(a.accessToken)
}
