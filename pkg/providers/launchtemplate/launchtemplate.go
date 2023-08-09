/*
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

package launchtemplate

import (
	"context"
	"strings"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/gpu-vmprovisioner/pkg/providers/imagefamily"
	"github.com/gpu-vmprovisioner/pkg/providers/launchtemplate/parameters"

	"github.com/samber/lo"

	"github.com/gpu-vmprovisioner/pkg/apis/settings"
	"github.com/gpu-vmprovisioner/pkg/apis/v1alpha1"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"

	"github.com/aws/karpenter-core/pkg/cloudprovider"
)

const (
	karpenterManagedTagKey = "karpenter.k8s.azure/cluster"
)

type Template struct {
	UserData string
	ImageID  string
	Tags     map[string]*string
}

type Provider struct {
	imageFamily            *imagefamily.Resolver
	caBundle               *string
	clusterEndpoint        string
	tenantID               string
	subscriptionID         string
	userAssignedIdentityID string
	resourceGroup          string
	location               string
}

// TODO: add caching of launch templates

func NewProvider(_ context.Context, imageFamily *imagefamily.Resolver, caBundle *string, clusterEndpoint string,
	tenantID, subscriptionID, userAssignedIdentityID, resourceGroup, location string,
) *Provider {
	l := &Provider{
		imageFamily:            imageFamily,
		caBundle:               caBundle,
		clusterEndpoint:        clusterEndpoint,
		tenantID:               tenantID,
		subscriptionID:         subscriptionID,
		userAssignedIdentityID: userAssignedIdentityID,
		resourceGroup:          resourceGroup,
		location:               location,
	}
	return l
}

func (p *Provider) GetTemplate(ctx context.Context, nodeTemplate *v1alpha1.NodeTemplate, machine *v1alpha5.Machine,
	instanceType *cloudprovider.InstanceType, additionalLabels map[string]string) (*Template, error) {
	// TODO: add caching of launch templates, based on static parameters
	staticParameters := p.getStaticParameters(ctx, nodeTemplate, lo.Assign(machine.Labels, additionalLabels))
	templateParameters, err := p.imageFamily.Resolve(ctx, nodeTemplate, machine, instanceType, staticParameters)
	if err != nil {
		return nil, err
	}
	launchTemplate, err := p.createLaunchTemplate(ctx, templateParameters)
	if err != nil {
		return nil, err
	}

	return launchTemplate, nil
}

func (p *Provider) getStaticParameters(ctx context.Context, nodeTemplate *v1alpha1.NodeTemplate, labels map[string]string) *parameters.StaticParameters {
	return &parameters.StaticParameters{
		ClusterName:     settings.FromContext(ctx).ClusterName,
		ClusterEndpoint: p.clusterEndpoint,
		Tags:            lo.Assign(settings.FromContext(ctx).Tags, nodeTemplate.Spec.Tags),
		Labels:          labels,
		CABundle:        p.caBundle,

		TenantID:                       p.tenantID,
		SubscriptionID:                 p.subscriptionID,
		UserAssignedIdentityID:         p.userAssignedIdentityID,
		ResourceGroup:                  p.resourceGroup,
		Location:                       p.location,
		ClusterID:                      settings.FromContext(ctx).ClusterID,
		APIServerName:                  settings.FromContext(ctx).GetAPIServerName(),
		KubeletClientTLSBootstrapToken: settings.FromContext(ctx).KubeletClientTLSBootstrapToken,
		NetworkPlugin:                  settings.FromContext(ctx).NetworkPlugin,
		NetworkPolicy:                  settings.FromContext(ctx).NetworkPolicy,
		// KubernetesVersion:           - currently set in .Resolve
	}
}

func (p *Provider) createLaunchTemplate(_ context.Context, options *parameters.Parameters) (*Template, error) {
	// render user data
	userData, err := options.UserData.Script()
	if err != nil {
		return nil, err
	}

	// merge and convert to ARM tags
	azureTags := mergeTags(options.Tags, map[string]string{karpenterManagedTagKey: options.ClusterName})
	template := &Template{
		UserData: userData,
		ImageID:  options.ImageID,
		Tags:     azureTags,
	}
	return template, nil
}

// MergeTags takes a variadic list of maps and merges them together
// with format acceptable to ARM (no / in keys, pointer to strings as values)
func mergeTags(tags ...map[string]string) (result map[string]*string) {
	return lo.MapEntries(lo.Assign(tags...), func(key string, value string) (string, *string) {
		return strings.ReplaceAll(key, "/", "_"), to.StringPtr(value)
	})
}
