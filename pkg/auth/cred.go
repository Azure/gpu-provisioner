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
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
)

// ClientAssertionCredential authenticates an application with assertions provided by a callback function.
type ClientAssertionCredential struct {
	assertion, file string
	client          confidential.Client
	lastRead        time.Time
}

// NewCredential provides a token credential for msi and service principal auth
func NewCredential(cfg *Config, env *azure.Environment) (azcore.TokenCredential, error) {
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

	cred := confidential.NewCredFromAssertionCallback(
		func(ctx context.Context, _ confidential.AssertionRequestOptions) (string, error) {
			return c.readJWTFromFS()
		},
	)

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
