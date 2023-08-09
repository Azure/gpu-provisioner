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

	core "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gpu-vmprovisioner/pkg/apis/v1alpha1"
	"github.com/gpu-vmprovisioner/pkg/providers/imagefamily/bootstrap"
	template "github.com/gpu-vmprovisioner/pkg/providers/launchtemplate/parameters"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
)

// Resolver is able to fill-in dynamic launch template parameters
type Resolver struct {
	imageProvider *Provider
}

// ImageFamily can be implemented to override the default logic for generating dynamic launch template parameters
type ImageFamily interface {
	UserData(
		kubeletConfig *v1alpha5.KubeletConfiguration,
		taints []core.Taint,
		labels map[string]string,
		caBundle *string,
		instanceType *cloudprovider.InstanceType,
		customUserData *string,
	) bootstrap.Bootstrapper
}

// New constructs a new launch template Resolver
func New(_ client.Client, imageProvider *Provider) *Resolver {
	return &Resolver{
		imageProvider: imageProvider,
	}
}

// Resolve fills in dynamic launch template parameters
func (r Resolver) Resolve(ctx context.Context, nodeTemplate *v1alpha1.NodeTemplate, machine *v1alpha5.Machine, instanceType *cloudprovider.InstanceType,
	staticParameters *template.StaticParameters) (*template.Parameters, error) {
	// TODO: move to launch template provider; don't change staticParameters here
	kubeServerVersion, err := r.imageProvider.KubeServerVersion(ctx)
	if err != nil {
		return nil, err
	}
	staticParameters.KubernetesVersion = kubeServerVersion
	// TODO: support specifying image family in node template
	// imageFamily := getImageFamily(nodeTemplate.Spec.ImageFamily, options)
	imageFamily := getImageFamily(staticParameters)
	imageID, err := r.imageProvider.Get(ctx, nodeTemplate, instanceType, imageFamily)
	if err != nil {
		return nil, err
	}

	template := &template.Parameters{
		StaticParameters: staticParameters,
		UserData: imageFamily.UserData(
			machine.Spec.Kubelet,
			append(machine.Spec.Taints, machine.Spec.StartupTaints...),
			staticParameters.Labels,
			staticParameters.CABundle,
			instanceType,
			nil, //  nodeTemplate.Spec.UserData,
		),
		ImageID: imageID,
	}

	return template, nil
}

func getImageFamily(parameters *template.StaticParameters) ImageFamily {
	// TODO: support other image families
	return &Ubuntu{Options: parameters}
}
