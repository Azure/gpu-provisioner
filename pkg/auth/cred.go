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
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/2016-10-01/keyvault"
	kvauth "github.com/Azure/azure-sdk-for-go/services/keyvault/auth"
	"github.com/Azure/go-autorest/autorest"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"
	"github.com/azure/gpu-provisioner/pkg/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/http2"
	"k8s.io/klog/v2"
)

const (
	e2eOverlayResourceVersionKey       = "AKS_E2E_OVERLAY_RESOURCE_VERSION"
	E2E_RP_INGRESS_ENDPOINT_ADDRESS    = "rp.e2e.ig.e2e-aks.azure.com"
	E2E_SERVICE_CONFIGURATION_AUDIENCE = "https://management.core.windows.net/"
	HTTPSPrefix                        = "https://"
	E2E_RP_INGRESS_ENDPOINT            = "rp.e2e.ig.e2e-aks.azure.com"
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
	isE2E := utils.WithDefaultBool("E2E_TEST_MODE", false)
	if isE2E {
		dummy := DummyCredential{}
		token, err := dummy.GetToken(ctx, tokenOpt)
		if err != nil {
			return nil, err
		}
		credAuth.TokenCredential = token
		return credAuth, nil
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
		token, err := credAuth.GetToken(ctx, tokenOpt)
		if err != nil {
			klog.ErrorS(err, "failed to acquire token")
			return &CredentialAuth{}, errors.Wrap(err, "failed to acquire token")
		}
		klog.Infof("token: %s", token)
		credAuth.TokenCredential = token
		credAuth.Authorizer = autorest.NewBearerAuthorizer(credAuth)
		klog.Infof("credAuth.Authorizer.WithAuthorization(): %p", credAuth.Authorizer.WithAuthorization())
	}

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

type E2ELogfFunc func(message string, args ...interface{})

func BuildHTTPClient(ctx context.Context) (*http.Client, error) {
	armClientCert, err := getE2ETestingCert(ctx)
	if err != nil {
		return nil, err
	}

	certPEM, keyPEM := SplitPEMBlock([]byte(armClientCert))
	if len(certPEM) == 0 {
		return nil, errors.New("malformed cert pem format")
	}

	// Load client cert
	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM([]byte(certPEM))
	if !ok {
		return nil, errors.New("")
	}
	// Setup HTTPS client
	// #nosec G402 the https cert in e2e is a private cert
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == E2E_RP_INGRESS_ENDPOINT_ADDRESS {
				addr = fmt.Sprintf("%s%s", fmt.Sprintf("aksrpingress-e2e-%s.%s.cloudapp.azure.com", "heelayotebld95054108", "eastus"), ":443")
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

// if no frame is received for 30s, the transport will issue a ping health check to the server.
const http2ReadIdleTimeout = 30 * time.Second

// we give 10s to the server to respond to the ping. if no response is received,
// the transport will close the connection, so that the next request will open a new connection, and not
// hit a context deadline exceeded error.
const http2PingTimeout = 10 * time.Second

// configureHTTP2Transport ensures that our defaultTransport is configured
// with the http2 additional settings that work around the issue described here:
// https://github.com/golang/go/issues/59690
// azure sdk related issue is here:
// https://github.com/Azure/azure-sdk-for-go/issues/21346#issuecomment-1699665586
// This is called by the package init to ensure that our defaultTransport is always configured
// you should not call this anywhere else.
func configureHTTP2Transport(t *http.Transport) error {
	// htt2Trans holds a reference to the default transport and configures "h2" middlewares that
	// will use the below settings, making the standard http.Transport behave correctly.
	http2Trans, err := http2.ConfigureTransports(t)
	if err == nil {
		// if no frame is received for 30s, the transport will issue a ping health check to the server.
		http2Trans.ReadIdleTimeout = http2ReadIdleTimeout
		// we give 10s to the server to respond to the ping. if no response is received,
		// the transport will close the connection, so that the next request will open a new connection, and not
		// hit a context deadline exceeded error.
		http2Trans.PingTimeout = http2PingTimeout
	}
	return err
}

// split the pem block to cert/key
func SplitPEMBlock(pemBlock []byte) (certPEM []byte, keyPEM []byte) {
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

func CloneCloudConfiguration(cloudConfig *cloud.Configuration) *cloud.Configuration {
	clone := *cloudConfig
	clone.Services = make(map[cloud.ServiceName]cloud.ServiceConfiguration)
	for k, v := range cloudConfig.Services {
		clone.Services[k] = v
	}
	return &clone
}
