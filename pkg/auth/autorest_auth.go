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

	"github.com/Azure/go-autorest/autorest"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

// authResult contains the subset of results from a token acquisition operation in ConfidentialClientApplication
// For details see https://aka.ms/msal-net-authenticationresult
type authResult struct {
	accessToken    string
	expiresOn      time.Time
	grantedScopes  []string
	declinedScopes []string
}

func NewAuthorizer(ctx context.Context, config *Config, resourceEndpoint string) (autorest.Authorizer, error) {

	// Azure AD Workload Identity webhook will inject the following env vars:
	// 	AZURE_FEDERATED_TOKEN_FILE is the service account token path
	// 	AZURE_AUTHORITY_HOST is the AAD authority hostname

	tokenFilePath := os.Getenv("AZURE_FEDERATED_TOKEN_FILE")
	authority := os.Getenv("AZURE_AUTHORITY_HOST")

	if tokenFilePath == "" || authority == "" {
		return nil, fmt.Errorf("required environment variables not set, AZURE_FEDERATED_TOKEN_FILE: %s, AZURE_AUTHORITY_HOST: %s", tokenFilePath, authority)
	}

	cred := confidential.NewCredFromAssertionCallback(func(context.Context, confidential.AssertionRequestOptions) (string, error) {
		return readJWTFromFS(tokenFilePath)
	})
	// create the confidential client to request an AAD token
	confidentialClientApp, err := confidential.New(
		fmt.Sprintf("%s%s/oauth2/token", authority, config.TenantID),
		config.UserAssignedIdentityID,
		cred)
	if err != nil {
		return nil, fmt.Errorf("failed to create confidential client app: %w", err)
	}

	result, err := confidentialClientApp.AcquireTokenByCredential(
		ctx,
		[]string{strings.TrimSuffix(resourceEndpoint, "/") + "/.default"})
	if err != nil {
		klog.ErrorS(err, "failed to acquire token")
		return autorest.NewBearerAuthorizer(authResult{}), errors.Wrap(err, "failed to acquire token")
	}

	return autorest.NewBearerAuthorizer(authResult{
		accessToken:    result.AccessToken,
		expiresOn:      result.ExpiresOn,
		grantedScopes:  result.GrantedScopes,
		declinedScopes: result.DeclinedScopes,
	}), nil
}

// OAuthToken implements the OAuthTokenProvider interface.  It returns the current access token.
func (ar authResult) OAuthToken() string {
	return ar.accessToken
}

func (a *authResult) WithAuthorization() autorest.PrepareDecorator {
	return autorest.WithBearerAuthorization(a.accessToken)
}

// readJWTFromFS reads the jwt from a file system
func readJWTFromFS(tokenFilePath string) (string, error) {
	token, err := os.ReadFile(tokenFilePath)
	if err != nil {
		return "", err
	}
	return string(token), nil
}
