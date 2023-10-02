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

package test

import (
	"encoding/json"
	"fmt"

	"github.com/imdario/mergo"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
)

// ProvisionerOptions customizes a Provisioner.
type ProvisionerOptions struct {
	metav1.ObjectMeta
	Limits                 v1.ResourceList
	Provider               interface{}
	ProviderRef            *v1alpha5.MachineTemplateRef
	Kubelet                *v1alpha5.KubeletConfiguration
	Annotations            map[string]string
	Labels                 map[string]string
	Taints                 []v1.Taint
	StartupTaints          []v1.Taint
	Requirements           []v1.NodeSelectorRequirement
	Status                 v1alpha5.ProvisionerStatus
	TTLSecondsUntilExpired *int64
	Weight                 *int32
	TTLSecondsAfterEmpty   *int64
	Consolidation          *v1alpha5.Consolidation
}

// Provisioner creates a test provisioner with defaults that can be overridden by ProvisionerOptions.
// Overrides are applied in order, with a last write wins semantic.
func Provisioner(overrides ...ProvisionerOptions) *v1alpha5.Provisioner {
	options := ProvisionerOptions{}
	for _, opts := range overrides {
		if err := mergo.Merge(&options, opts, mergo.WithOverride); err != nil {
			panic(fmt.Sprintf("Failed to merge provisioner options: %s", err))
		}
	}
	if options.Name == "" {
		options.Name = RandomName()
	}
	if options.Limits == nil {
		options.Limits = v1.ResourceList{v1.ResourceCPU: resource.MustParse("2000")}
	}
	raw := &runtime.RawExtension{}
	lo.Must0(raw.UnmarshalJSON(lo.Must(json.Marshal(options.Provider))))

	provisioner := &v1alpha5.Provisioner{
		ObjectMeta: ObjectMeta(options.ObjectMeta),
		Spec: v1alpha5.ProvisionerSpec{
			Requirements:           options.Requirements,
			KubeletConfiguration:   options.Kubelet,
			ProviderRef:            options.ProviderRef,
			Taints:                 options.Taints,
			StartupTaints:          options.StartupTaints,
			Annotations:            options.Annotations,
			Labels:                 lo.Assign(options.Labels, map[string]string{DiscoveryLabel: "unspecified"}), // For node cleanup discovery
			Limits:                 &v1alpha5.Limits{Resources: options.Limits},
			TTLSecondsAfterEmpty:   options.TTLSecondsAfterEmpty,
			TTLSecondsUntilExpired: options.TTLSecondsUntilExpired,
			Weight:                 options.Weight,
			Consolidation:          options.Consolidation,
			Provider:               raw,
		},
		Status: options.Status,
	}

	if options.ProviderRef == nil {
		if options.Provider == nil {
			options.Provider = struct{}{}
		}
		provider, err := json.Marshal(options.Provider)
		if err != nil {
			panic(err.Error())
		}
		provisioner.Spec.Provider = &runtime.RawExtension{Raw: provider}
	}
	return provisioner
}
