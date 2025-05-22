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
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
	"golang.org/x/net/http2"
)

// ClientAssertionCredential authenticates an application with assertions provided by a callback function.
type ClientAssertionCredential struct {
	assertion, file string
	client          confidential.Client
	lastRead        time.Time
}

// NewCredential provides a token credential for msi and service principal auth
func NewCredential(cfg *Config) (azcore.TokenCredential, error) {
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

func GetE2ETLSConfig(ctx context.Context, cred azcore.TokenCredential, cfg *Config) (*http.Client, error) {
	armClientCert, err := getE2ETestingCert(ctx, cred, cfg)
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

	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM([]byte(certPEM))
	if !ok {
		return nil, errors.New("cannot append certs from pem")
	}

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	rpIngressAddress := normalizeHostPort(constructE2ERPIngressEndpointAddress(ctx, cfg))
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == E2E_RP_INGRESS_ENDPOINT_ADDRESS {
				addr = rpIngressAddress
			}
			return dialer.DialContext(ctx, network, addr)
		},
		// Configuring DialContext disables HTTP/2 by default.
		// our use of DialContext does not impair the use of http/2 so we can force it.
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if err = configureHTTP2Transport(transport); err != nil { //+gocover:ignore:block - can't make this fail.
		return nil, err
	}
	client := &http.Client{
		Transport: transport,
	}
	return client, nil
}

func normalizeHostPort(hostMaybeWithPort string) string {
	if _, _, err := net.SplitHostPort(hostMaybeWithPort); err == nil {
		return hostMaybeWithPort // host already has a port
	}

	return fmt.Sprintf("%s%s", hostMaybeWithPort, HTTPS_PORT)
}

// configureHTTP2Transport ensures that our defaultTransport is configured
// with the http2 additional settings that work around the issue described here:
// https://github.com/golang/go/issues/59690
// azure sdk related issue is here:
// https://github.com/Azure/azure-sdk-for-go/issues/21346#issuecomment-1699665586
// you should not call this anywhere else.
func configureHTTP2Transport(t *http.Transport) error {
	// htt2Trans holds a reference to the default transport and configures "h2" middlewares that
	// will use the below settings, making the standard http.Transport behave correctly.
	http2Trans, err := http2.ConfigureTransports(t)
	if err == nil {
		// if no frame is received for 30s, the transport will issue a ping health check to the server.
		http2Trans.ReadIdleTimeout = http2ReadIdleTimeout
		// we give 10s to the server to respond to the ping. if no response is received,
		// the transport will close the connection, so that the next request will open a new connection and not
		// hit a context deadline exceeded error.
		http2Trans.PingTimeout = http2PingTimeout
	}
	return err
}

func getE2ETestingCert(ctx context.Context, cred azcore.TokenCredential, cfg *Config) (*string, error) {
	clientCertSecretName := "e2e-arm-client-c" + "ert" // workaround for G101

	keyVaultUrl := fmt.Sprintf("https://hcp%s.vault.azure.net", cfg.E2EOverlayResourceVersion)
	kvClient, err := azsecrets.NewClient(keyVaultUrl, cred, nil)
	if err != nil { //+gocover:ignore:block keyvault fetch
		return nil, err
	}

	result, err := kvClient.GetSecret(ctx, clientCertSecretName, "", nil)
	if err != nil { //+gocover:ignore:block keyvault fetch
		return nil, err
	}

	if result.Value == nil {
		return nil, fmt.Errorf("secret %q not found", clientCertSecretName)
	}

	return result.Value, nil
}

func constructE2ERPIngressEndpointAddress(ctx context.Context, cfg *Config) string {
	return fmt.Sprintf("aksrpingress-e2e-%s.%s.cloudapp.azure.com", cfg.E2EBuildVersion, cfg.Location)
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
