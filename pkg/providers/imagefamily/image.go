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

package imagefamily

import (
	"context"
	"strings"

	"github.com/Azure/karpenter/pkg/apis/v1alpha1"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/utils/pretty"
	"github.com/patrickmn/go-cache"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/logging"
)

type Provider struct {
	kubernetesVersionCache *cache.Cache
	cm                     *pretty.ChangeMonitor
	kubernetesInterface    kubernetes.Interface
}

type Image struct {
	ImageID      string
	CreationDate string
}

const (
	kubernetesVersionCacheKey = "kubernetesVersion"

	// Default Image
	DefaultImageID = "/CommunityGalleries/akstestgallery-ab568100-7a05-4b31-9f7d-3aede6df4420/images/1804Gen2/versions/1.1676428672.1273"
)

func NewProvider(kubernetesInterface kubernetes.Interface, kubernetesVersionCache *cache.Cache) *Provider {
	return &Provider{
		kubernetesVersionCache: kubernetesVersionCache,
		cm:                     pretty.NewChangeMonitor(),
		kubernetesInterface:    kubernetesInterface,
	}
}

// Get returns Image ID for the given instance type. Images may vary due to architecture, accelerator, etc
func (p *Provider) Get(_ context.Context, nodeTemplate *v1alpha1.NodeTemplate, _ *cloudprovider.InstanceType, _ ImageFamily) (string, error) {
	// TODO: support selecting images based on image type requirements
	// TODO: remove hardcoded image ID

	/*  sample (outdated) logic
	// prefer gen2 image for compatible SKUs
	hyperv2requirements := scheduling.NewRequirements(scheduling.NewRequirement(v1alpha1.LabelSKUHyperVGeneration, v1.NodeSelectorOpIn, "V2"))
	if err := instanceType.Requirements.Compatible(hyperv2requirements); err == nil {
		return to.StringPtr("aks-ubuntu-1804-gen2-2022-q1")
	}
	return to.StringPtr("aks-ubuntu-1804-2022-q1")
	*/
	if !nodeTemplate.Spec.IsEmptyImageID() {
		return *nodeTemplate.Spec.ImageID, nil
	}
	return DefaultImageID, nil
}

func (p *Provider) KubeServerVersion(ctx context.Context) (string, error) {
	if version, ok := p.kubernetesVersionCache.Get(kubernetesVersionCacheKey); ok {
		return version.(string), nil
	}
	serverVersion, err := p.kubernetesInterface.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	version := strings.TrimPrefix(serverVersion.GitVersion, "v") // v1.24.9 -> 1.24.9
	p.kubernetesVersionCache.SetDefault(kubernetesVersionCacheKey, version)
	if p.cm.HasChanged("kubernetes-version", version) {
		logging.FromContext(ctx).With("kubernetes-version", version).Debugf("discovered kubernetes version")
	}
	return version, nil
}
