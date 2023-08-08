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

package bootstrap

import (
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/ptr"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/utils/resources"
)

// Options is the node bootstrapping parameters passed from Karpenter to the provisioning node
type Options struct {
	ClusterName             string
	ClusterEndpoint         string
	KubeletConfig           *v1alpha5.KubeletConfiguration
	Taints                  []core.Taint      `hash:"set"`
	Labels                  map[string]string `hash:"set"`
	CABundle                *string
	AWSENILimitedPodDensity bool
	ContainerRuntime        *string
	CustomUserData          *string
}

func (o Options) kubeletExtraArgs() (args []string) {
	args = append(args, o.nodeLabelArg(), o.nodeTaintArg())

	if o.KubeletConfig == nil {
		return lo.Compact(args)
	}
	if o.KubeletConfig.MaxPods != nil {
		args = append(args, fmt.Sprintf("--max-pods=%d", ptr.Int32Value(o.KubeletConfig.MaxPods)))
	}
	if o.KubeletConfig.PodsPerCore != nil {
		args = append(args, fmt.Sprintf("--pods-per-core=%d", ptr.Int32Value(o.KubeletConfig.PodsPerCore)))
	}
	// We have to convert some of these maps so that their values return the correct string
	args = append(args, joinParameterArgs("--system-reserved", resources.StringMap(o.KubeletConfig.SystemReserved), "="))
	args = append(args, joinParameterArgs("--kube-reserved", resources.StringMap(o.KubeletConfig.KubeReserved), "="))
	args = append(args, joinParameterArgs("--eviction-hard", o.KubeletConfig.EvictionHard, "<"))
	args = append(args, joinParameterArgs("--eviction-soft", o.KubeletConfig.EvictionSoft, "<"))
	args = append(args, joinParameterArgs("--eviction-soft-grace-period", lo.MapValues(o.KubeletConfig.EvictionSoftGracePeriod, func(v metav1.Duration, _ string) string { return v.Duration.String() }), "="))

	if o.KubeletConfig.EvictionMaxPodGracePeriod != nil {
		args = append(args, fmt.Sprintf("--eviction-max-pod-grace-period=%d", ptr.Int32Value(o.KubeletConfig.EvictionMaxPodGracePeriod)))
	}
	if o.KubeletConfig.ImageGCHighThresholdPercent != nil {
		args = append(args, fmt.Sprintf("--image-gc-high-threshold=%d", ptr.Int32Value(o.KubeletConfig.ImageGCHighThresholdPercent)))
	}
	if o.KubeletConfig.ImageGCLowThresholdPercent != nil {
		args = append(args, fmt.Sprintf("--image-gc-low-threshold=%d", ptr.Int32Value(o.KubeletConfig.ImageGCLowThresholdPercent)))
	}
	if o.KubeletConfig.CPUCFSQuota != nil {
		args = append(args, fmt.Sprintf("--cpu-cfs-quota=%t", lo.FromPtr(o.KubeletConfig.CPUCFSQuota)))
	}
	return lo.Compact(args)
}

func (o Options) nodeTaintArg() string {
	if len(o.Taints) == 0 {
		return ""
	}
	var taintStrings []string
	for _, taint := range o.Taints {
		taintStrings = append(taintStrings, fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect))
	}
	return fmt.Sprintf("--register-with-taints=%q", strings.Join(taintStrings, ","))
}

func (o Options) nodeLabelArg() string {
	if len(o.Labels) == 0 {
		return ""
	}
	var labelStrings []string
	keys := lo.Keys(o.Labels)
	sort.Strings(keys) // ensures this list is deterministic, for easy testing.
	for _, key := range keys {
		if v1alpha5.LabelDomainExceptions.Has(key) {
			continue
		}
		labelStrings = append(labelStrings, fmt.Sprintf("%s=%v", key, o.Labels[key]))
	}
	return fmt.Sprintf("--node-labels=%q", strings.Join(labelStrings, ","))
}

// joinParameterArgs joins a map of keys and values by their separator. The separator will sit between the
// arguments in a comma-separated list i.e. arg1<sep>val1,arg2<sep>val2
func joinParameterArgs[K comparable, V any](name string, m map[K]V, separator string) string {
	var args []string

	for k, v := range m {
		args = append(args, fmt.Sprintf("%v%s%v", k, separator, v))
	}
	if len(args) > 0 {
		return fmt.Sprintf("%s=%q", name, strings.Join(args, ","))
	}
	return ""
}

// Bootstrapper can be implemented to generate a bootstrap script
// that uses the params from the Bootstrap type for a specific
// bootstrapping method.
// Examples are the Bottlerocket config and the eks-bootstrap script
type Bootstrapper interface {
	Script() (string, error)
}
