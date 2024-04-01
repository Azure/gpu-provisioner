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
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/2016-10-01/keyvault"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
	"github.com/azure/gpu-provisioner/pkg/utils"

	"github.com/pkg/errors"
)

const (
	e2eOverlayResourceVersionKey = "AKS_E2E_OVERLAY_RESOURCE_VERSION"
)

// ClientAssertionCredential authenticates an application with assertions provided by a callback function.
type ClientAssertionCredential struct {
	assertion, file string
	client          confidential.Client
	lastRead        time.Time
}

// NewCredential provides a token credential for msi and service principal auth
func NewCredential(cfg *Config, authorizer autorest.Authorizer) (azcore.TokenCredential, error) {
	if cfg == nil {
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
	c := &ClientAssertionCredential{file: tokenFilePath}

	var cred confidential.Credential
	isE2E := utils.WithDefaultBool("E2E_TEST_MODE", false)
	if isE2E {
		armClientCert, err := getE2ETestingCert(authorizer)
		if err != nil {
			return nil, err
		}
		certPEM, keyPEM := splitPEMBlock([]byte(to.String(armClientCert)))
		if len(certPEM) == 0 {
			return nil, errors.New("malformed cert pem format")
		}

		// Load client cert
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, err
		}
		leafCert := []tls.Certificate{cert}
		cred, err = confidential.NewCredFromCert([]*x509.Certificate{leafCert[0].Leaf}, keyPEM)
		if err != nil {
			return nil, err
		}
	} else {
		cred = confidential.NewCredFromAssertionCallback(
			func(ctx context.Context, _ confidential.AssertionRequestOptions) (string, error) {
				return c.readJWTFromFS()
			},
		)
	}

	// create the confidential client to request an AAD token
	confidentialClientApp, err := confidential.New(
		fmt.Sprintf("%s%s/oauth2/token", authority, cfg.TenantID),
		cfg.UserAssignedIdentityID,
		cred)
	if err != nil {
		return nil, fmt.Errorf("failed to create confidential client app: %w", err)
	}
	c.client = confidentialClientApp

	return c, nil
}

// GetToken implements the TokenCredential interface
func (c *ClientAssertionCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	// get the token from the confidential client
	token, err := c.client.AcquireTokenByCredential(ctx, opts.Scopes)
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

func getE2ETestingCert(authorizer autorest.Authorizer) (*string, error) {
	e2eOverlayResourceVersion := os.Getenv(e2eOverlayResourceVersionKey)
	if e2eOverlayResourceVersion == "" {
		return nil, fmt.Errorf("E2E overlay resource version is not set")
	}

	keyVaultUrl := fmt.Sprintf("https://hcp%s.vault.azure.net/", e2eOverlayResourceVersion)
	client := keyvault.New()
	client.Authorizer = authorizer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := client.GetSecret(ctx, keyVaultUrl, "e2e-arm-client-cert", "")
	if err != nil { //+gocover:ignore:block keyvault fetch
		return nil, err
	}
	return result.Value, nil
}

// split the pem block to cert/key
func splitPEMBlock(pemBlock []byte) (certPEM []byte, keyPEM []byte) {
	for {
		var derBlock *pem.Block
		derBlock, pemBlock = pem.Decode(pemBlock)
		if derBlock == nil {
			break
		}
		if derBlock.Type == "CERTIFICATE" {
			certPEM = append(certPEM, pem.EncodeToMemory(derBlock)...)
		} else if derBlock.Type == "PRIVATE KEY" {
			keyPEM = append(keyPEM, pem.EncodeToMemory(derBlock)...)
		}
	}
	return certPEM, keyPEM
}
