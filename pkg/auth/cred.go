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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/2016-10-01/keyvault"
	kvauth "github.com/Azure/azure-sdk-for-go/services/keyvault/auth"
	"github.com/Azure/go-autorest/autorest"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
	"github.com/azure/gpu-provisioner/pkg/utils"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

const (
	e2eOverlayResourceVersionKey = "AKS_E2E_OVERLAY_RESOURCE_VERSION"
)

// CredentialAuth authenticates an application with assertions provided by a callback function.
type CredentialAuth struct {
	assertion, authFile string
	client              confidential.Client
	Authorizer          autorest.Authorizer
	TokenCredential     azcore.AccessToken
	lastRead            time.Time
}

// NewCredentialAuth provides a token credential for msi and service principal auth
func NewCredentialAuth(ctx context.Context, config *Config) (*CredentialAuth, error) {
	klog.Infof("NewCredentialAuth: %v", config)
	if config == nil {
		return nil, fmt.Errorf("failed to create credential, nil config provided")
	}

	// Azure AD Workload Identity webhook will inject the following env vars:
	// 	AZURE_FEDERATED_TOKEN_FILE is the service account token path
	// 	AZURE_AUTHORITY_HOST is the AAD authority hostname

	tokenFilePath := os.Getenv("AZURE_FEDERATED_TOKEN_FILE")
	authority := os.Getenv("AZURE_AUTHORITY_HOST")

	if tokenFilePath == "" || authority == "" {
		return nil, fmt.Errorf("required environment variables not set, AZURE_FEDERATED_TOKEN_FILE: %s, AZURE_AUTHORITY_HOST: %s", tokenFilePath, authority)
	}
	credAuth := &CredentialAuth{authFile: tokenFilePath}

	tokenOpt := policy.TokenRequestOptions{
		Scopes: []string{strings.TrimSuffix(config.getCloudConfiguration().Services[cloud.ResourceManager].Endpoint, "/") + "/.default"},
	}
	var token azcore.AccessToken
	isE2E := utils.WithDefaultBool("E2E_TEST_MODE", false)
	if isE2E {
		armClientCert, err := getE2ETestingCert(ctx)
		if err != nil {
			return nil, err
		}

		cert, privateKey, err := azidentity.ParseCertificates([]byte(armClientCert), nil)
		if err != nil {
			klog.Fatal(err)
		}
		ClientCred, err := azidentity.NewClientCertificateCredential(config.TenantID, config.UserAssignedIdentityID, cert, privateKey, &azidentity.ClientCertificateCredentialOptions{
			SendCertificateChain: true,
		})
		if err != nil {
			return nil, err
		}

		token, err = ClientCred.GetToken(ctx, tokenOpt)
		if err != nil {
			klog.ErrorS(err, "failed to acquire token")
			return &CredentialAuth{}, errors.Wrap(err, "failed to acquire token")
		}
		klog.Infof("token: %s", token)
		credAuth.TokenCredential = token
		//cert, privateKey, err := confidential.CertFromPEM([]byte(armClientCert), "")
		//if err != nil {
		//	log.Fatal(err)
		//}
		//klog.Infof("privateKey: %s", privateKey)
		//cred, err = confidential.NewCredFromCert(cert, privateKey)
		//if err != nil {
		//	return nil, err
		//}
	} else {
		cred := confidential.NewCredFromAssertionCallback(
			func(ctx context.Context, _ confidential.AssertionRequestOptions) (string, error) {
				return credAuth.readJWTFromFS()
			},
		)
		// create the confidential client to request an AAD token
		confidentialClientApp, err := confidential.New(
			fmt.Sprintf("%s%s/oauth2/token", authority, config.TenantID),
			config.UserAssignedIdentityID,
			cred)
		if err != nil {
			return nil, fmt.Errorf("failed to create confidential client app: %w", err)
		}
		// get the token and authorizer from the confidential client
		credAuth.client = confidentialClientApp
		token, err = credAuth.GetToken(ctx, tokenOpt)
		if err != nil {
			klog.ErrorS(err, "failed to acquire token")
			return &CredentialAuth{}, errors.Wrap(err, "failed to acquire token")
		}
		klog.Infof("token: %s", token)
		credAuth.TokenCredential = token
	}

	credAuth.Authorizer = autorest.NewBearerAuthorizer(credAuth)
	klog.Infof("credAuth.Authorizer.WithAuthorization(): %p", credAuth.Authorizer.WithAuthorization())

	return credAuth, nil
}

// OAuthToken implements the OAuthTokenProvider interface. It returns the current access token.
func (ca *CredentialAuth) OAuthToken() string {
	return ca.TokenCredential.Token
}

// GetToken implements the TokenCredential interface
func (ca *CredentialAuth) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	klog.Infof("GetToken")
	// get the token from the confidential client
	token, err := ca.client.AcquireTokenByCredential(ctx, opts.Scopes)
	if err != nil {
		return azcore.AccessToken{}, err
	}

	return azcore.AccessToken{
		Token:     token.AccessToken,
		ExpiresOn: token.ExpiresOn,
	}, nil
}

func (ca *CredentialAuth) WithAuthorization() autorest.PrepareDecorator {
	return autorest.WithBearerAuthorization(ca.TokenCredential.Token)
}

// readJWTFromFS reads the jwt from authFile system
// Source: https://github.com/Azure/azure-workload-identity/blob/d126293e3c7c669378b225ad1b1f29cf6af4e56d/examples/msal-go/token_credential.go#L88
func (ca *CredentialAuth) readJWTFromFS() (string, error) {
	klog.Infof("readJWTFromFS")
	if now := time.Now(); ca.lastRead.Add(5 * time.Minute).Before(now) {
		content, err := os.ReadFile(ca.authFile)
		if err != nil {
			return "", err
		}
		ca.assertion = string(content)
		ca.lastRead = now
	}
	return ca.assertion, nil
}
func getE2ETestingCert(ctx context.Context) (string, error) {
	klog.Info("getE2ETestingCert")
	e2eOverlayResourceVersion := "rscazghj6" //os.Getenv(e2eOverlayResourceVersionKey)
	if e2eOverlayResourceVersion == "" {
		return "", fmt.Errorf("E2E overlay resource version is not set")
	}

	keyVaultUrl := fmt.Sprintf("https://hcp%s.vault.azure.net/", e2eOverlayResourceVersion)

	client := keyvault.New()
	kvAuth, err := kvauth.NewAuthorizerFromEnvironment()
	if err != nil {
		return "", err
	}
	client.Authorizer = kvAuth

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	result, err1 := client.GetSecret(ctx, keyVaultUrl, "e2e-arm-client-cert", "")
	if err1 != nil {
		return "", err1
	}
	klog.Infof("Cert result: %s", *result.Value)
	return *result.Value, nil
}
