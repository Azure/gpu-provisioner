package instance

import (
	"context"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/azure/gpu-provisioner/pkg/auth"
)

const HeaderAKSHTTPCustomFeatures = "AKSHTTPCustomFeatures"

func prepareClientOptions(ctx context.Context) *arm.ClientOptions {
	optionsToUse := &arm.ClientOptions{}

	e2eCloudConfig := auth.CloneCloudConfiguration(&cloud.AzurePublic)
	e2eCloudConfig.Services[cloud.ResourceManager] = cloud.ServiceConfiguration{
		Audience: auth.E2E_SERVICE_CONFIGURATION_AUDIENCE,
		Endpoint: auth.HTTPSPrefix + auth.E2E_RP_INGRESS_ENDPOINT,
	}
	optionsToUse.ClientOptions.Cloud = *e2eCloudConfig

	features := []string{"Microsoft.ContainerService/AIToolchainOperatorPreview"}
	optionsToUse.PerCallPolicies = []policy.Policy{
		&InjectRefererPolicy{Referer: auth.HTTPSPrefix + auth.E2E_RP_INGRESS_ENDPOINT}, // set up referer header to make RP return the operation status query url based on https
		SetAKSFeaturesHeaderPolicy(false, true, features),                              // set up AKSHTTPCustomFeatures headers
	}

	optionsToUse.ClientOptions.PerCallPolicies = append(optionsToUse.ClientOptions.PerCallPolicies,
		PolicySetHeaders{
			"Referer": []string{auth.HTTPSPrefix + auth.E2E_RP_INGRESS_ENDPOINT},
		})
	return optionsToUse
}

func SetAKSFeaturesHeaderPolicy(setIfMissing, mergeExisting bool, features []string) PolicyFunc {
	return func(req *policy.Request) (*http.Response, error) {
		// remove duplicate
		featureMap := make(map[string]bool)
		for _, feature := range features {
			// remove empty string
			if strings.TrimSpace(feature) != "" {
				featureMap[feature] = true
			}
		}
		featureList := make([]string, 0, len(featureMap))
		for feature := range featureMap {
			featureList = append(featureList, feature)
		}
		setHeader(setIfMissing, mergeExisting, req.Raw(), HeaderAKSHTTPCustomFeatures, strings.Join(featureList, ","))
		return req.Next()
	}
}

func setHeader(setIfMissing, mergeExisting bool, r *http.Request, key, value string) {
	if r.Header == nil {
		r.Header = make(http.Header)
	}
	if len(r.Header.Values(key)) == 0 {
		r.Header.Set(key, value)
	} else {
		if mergeExisting {
			r.Header.Set(key, strings.Join(r.Header.Values(key), ","))
		}

		if setIfMissing {
			for _, v := range r.Header.Values(key) {
				for _, vv := range strings.Split(v, ",") {
					if strings.EqualFold(vv, value) {
						return
					}
				}
			}
		}
		if mergeExisting {
			r.Header.Set(key, strings.Join(r.Header.Values(key), ",")+","+value)
		} else {
			r.Header.Set(key, value)
		}
	}
}

// PolicySetHeaders sets http header
type PolicySetHeaders http.Header

func (p PolicySetHeaders) Do(req *policy.Request) (*http.Response, error) {
	header := req.Raw().Header
	for k, v := range p {
		header[k] = v
	}
	return req.Next()
}

type PolicyFunc func(*policy.Request) (*http.Response, error)

// Do implement the Policy interface on PolicyFunc.
func (pf PolicyFunc) Do(req *policy.Request) (*http.Response, error) {
	return pf(req)
}

// InjectRefererPolicy allows to add the referer header on all outgoing requests.
// this is used to set the async polling url HOST to a different one from the target URI.
type InjectRefererPolicy struct {
	Referer     string
	nextForTest func() (*http.Response, error)
}

func (p *InjectRefererPolicy) Do(req *policy.Request) (*http.Response, error) {
	req.Raw().Header.Add("Referer", p.Referer)
	return p.next(req)
}

// allows to fake the response in test
func (p *InjectRefererPolicy) next(req *policy.Request) (*http.Response, error) {
	if p.nextForTest == nil {
		return req.Next()
	}
	return p.nextForTest()
}
