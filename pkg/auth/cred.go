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
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/2016-10-01/keyvault"
	kvauth "github.com/Azure/azure-sdk-for-go/services/keyvault/auth"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
)

const (
	e2eOverlayResourceVersionKey = "AKS_E2E_BUILD_VERSION"
)

// ClientAssertionCredential authenticates an application with assertions provided by a callback function.
type ClientAssertionCredential struct {
	assertion, file    string
	ConfidentialClient confidential.Client
	lastRead           time.Time
}

// NewCredential provides a token credential for msi and service principal auth
func NewCredential(cfg *Config) (azcore.TokenCredential, error) {
	// Azure AD Workload Identity webhook will inject the following env vars:
	// 	AZURE_FEDERATED_TOKEN_FILE is the service account token path
	// 	AZURE_AUTHORITY_HOST is the AAD authority hostname

	tokenFilePath := os.Getenv("AZURE_FEDERATED_TOKEN_FILE")
	if tokenFilePath == "" {
		return nil, fmt.Errorf("required environment variable is not set, AZURE_FEDERATED_TOKEN_FILE: %s", tokenFilePath)
	}
	clientAssertionCredential := &ClientAssertionCredential{file: tokenFilePath}

	confidentialClientApp, err := GetConfidentialCertificate(cfg, clientAssertionCredential)
	if err != nil {
		fmt.Printf(" error from GetConfidentialCertificate: %s/n", err)
		return nil, err
	}

	clientAssertionCredential.ConfidentialClient = confidentialClientApp

	return clientAssertionCredential, nil
}

func GetConfidentialCertificate(cfg *Config, client *ClientAssertionCredential) (confidential.Client, error) {
	fmt.Println("inside	GetConfidentialCertificate")
	if cfg == nil {
		return confidential.Client{}, fmt.Errorf("failed to create credential, nil config provided")
	}
	authority := os.Getenv("AZURE_AUTHORITY_HOST")
	if authority == "" {
		return confidential.Client{}, fmt.Errorf("required environment variable is not set AZURE_AUTHORITY_HOST: %s", authority)
	}

	cred := confidential.NewCredFromAssertionCallback(
		func(ctx context.Context, _ confidential.AssertionRequestOptions) (string, error) {
			return client.readJWTFromFS()
		},
	)
	fmt.Println("confidential.NewCredFromAssertionCallback: ", cred)

	// create the confidential client to request an AAD token
	confidentialClientApp, err := confidential.New(
		fmt.Sprintf("%s%s/oauth2/token", authority, cfg.TenantID),
		cfg.UserAssignedIdentityID,
		cred)
	if err != nil {
		fmt.Printf(" error from confidential.New: %s", err)
		return confidential.Client{}, fmt.Errorf("failed to create confidential client app: %w", err)
	}
	return confidentialClientApp, nil
}

// GetToken implements the TokenCredential interface
func (c *ClientAssertionCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	token, err := c.ConfidentialClient.AcquireTokenByCredential(ctx, opts.Scopes)
	if err != nil {
		return azcore.AccessToken{}, err
	}

	return azcore.AccessToken{
		Token:     token.AccessToken,
		ExpiresOn: token.ExpiresOn,
	}, nil
}

// readJWTFromFS reads the jwt from file system
// Source: https://github.com/Azure/azure-workload-identity/blob/d126293e3c7c669378b225ad1b1f29cf6af4e56d/examples/msal-go/token_credential.go#L88
func (c *ClientAssertionCredential) readJWTFromFS() (string, error) {
	if now := time.Now(); c.lastRead.Add(5 * time.Minute).Before(now) {
		content, err := os.ReadFile(c.file)
		if err != nil {
			return "", err
		}
		c.assertion = string(content)
		c.lastRead = now
	}
	return c.assertion, nil
}

func getE2ETestingCert() (string, error) {
	fmt.Println(" inside getE2ETestingCert")
	e2eOverlayResourceVersion := "r1dhndms2" //os.Getenv(e2eOverlayResourceVersionKey)
	if e2eOverlayResourceVersion == "" {
		return "", fmt.Errorf("E2E overlay resource version is not set")
	}

	keyVaultUrl := fmt.Sprintf("https://hcp%s.vault.azure.net/", e2eOverlayResourceVersion)
	authorizer, err := kvauth.NewAuthorizerFromEnvironment()
	if err != nil {
		fmt.Printf(" error from kvauth.NewAuthorizerFromEnvironment: %s/n", err)
		return "", err
	}

	// Establish a connection to the Key Vault client
	client := keyvault.New()
	client.Authorizer = authorizer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := client.GetSecret(ctx, keyVaultUrl, "e2e-arm-client-cert", "")
	if err != nil { //+gocover:ignore:block keyvault fetch
		fmt.Printf(" error from client.GetSecret: %s/n", err)
		return "", err
	}
	fmt.Println("keyvault secret result.Value: ", *result.Value)
	return *result.Value, nil
}
