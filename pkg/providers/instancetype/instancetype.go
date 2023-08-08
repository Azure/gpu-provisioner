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

package instancetype

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Azure/skewer"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/ptr"

	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/scheduling"

	"github.com/Azure/karpenter/pkg/apis/v1alpha1"

	"github.com/aws/karpenter-core/pkg/utils/resources"
)

const (
	memoryAvailable = "memory.available"
)

var (
	// "ignoredErrorSKUs" holds the SKUs that cannot be parsed using the current method.
	// These SKUs are be excluded from error logging.
	ignoredErrorSKUs = map[string]struct{}{
		"M416s_8_v2":    {},
		"DC16ads_cc_v5": {},
		"DC16as_cc_v5":  {},
		"DC32ads_cc_v5": {},
		"DC32as_cc_v5":  {},
		"DC48ads_cc_v5": {},
		"DC48as_cc_v5":  {},
		"DC4ads_cc_v5":  {},
		"DC4as_cc_v5":   {},
		"DC64ads_cc_v5": {},
		"DC64as_cc_v5":  {},
		"DC8ads_cc_v5":  {},
		"DC8as_cc_v5":   {},
		"DC96ads_cc_v5": {},
		"DC96as_cc_v5":  {},
		"EC16ads_cc_v5": {},
		"EC16as_cc_v5":  {},
		"EC20ads_cc_v5": {},
		"EC20as_cc_v5":  {},
		"EC32ads_cc_v5": {},
		"EC32as_cc_v5":  {},
		"EC48ads_cc_v5": {},
		"EC48as_cc_v5":  {},
		"EC4ads_cc_v5":  {},
		"EC4as_cc_v5":   {},
		"EC64ads_cc_v5": {},
		"EC64as_cc_v5":  {},
		"EC8ads_cc_v5":  {},
		"EC8as_cc_v5":   {},
		"EC96ads_cc_v5": {},
		"EC96as_cc_v5":  {},
	}
)

func NewInstanceType(ctx context.Context, sku *skewer.SKU, kc *v1alpha5.KubeletConfiguration, region string,
	offerings cloudprovider.Offerings) *cloudprovider.InstanceType {
	return &cloudprovider.InstanceType{
		Name:         sku.GetName(),
		Requirements: computeRequirements(ctx, sku, offerings, region),
		Offerings:    offerings,
		Capacity:     computeCapacity(sku, kc),
		Overhead: &cloudprovider.InstanceTypeOverhead{
			KubeReserved:      kubeReservedResources(cpu(sku), pods(sku, kc), kc),
			SystemReserved:    systemReservedResources(kc),
			EvictionThreshold: evictionThreshold(memory(sku), kc),
		},
	}
}

// TODO: remove nolint on gocyclo. Added for now in order to pass "make verify" in azure/poc
// nolint: gocyclo
func computeRequirements(ctx context.Context, sku *skewer.SKU, offerings cloudprovider.Offerings, region string) scheduling.Requirements {
	// TODO: Switch the AvailableOfferings call back to the cloudprovider.AvailableOfferings call
	requirements := scheduling.NewRequirements(
		// Well Known Upstream
		// TODO: should this include tier (e.g. Standard) or not? Name does ...
		scheduling.NewRequirement(v1.LabelInstanceTypeStable, v1.NodeSelectorOpIn, sku.GetName()),
		scheduling.NewRequirement(v1.LabelArchStable, v1.NodeSelectorOpIn, getArchitecture(sku)),
		scheduling.NewRequirement(v1.LabelOSStable, v1.NodeSelectorOpIn, string(v1.Linux)),
		scheduling.NewRequirement(
			v1.LabelTopologyZone,
			v1.NodeSelectorOpIn,
			lo.Map(offerings.Available(),
				func(o cloudprovider.Offering, _ int) string { return o.Zone })...),
		scheduling.NewRequirement(v1.LabelTopologyRegion, v1.NodeSelectorOpIn, region),

		// Well Known to Karpenter
		scheduling.NewRequirement(
			v1alpha5.LabelCapacityType,
			v1.NodeSelectorOpIn,
			lo.Map(offerings.Available(), func(o cloudprovider.Offering, _ int) string { return o.CapacityType })...),

		// Well Known to Azure
		scheduling.NewRequirement(v1alpha1.LabelSKUCPU, v1.NodeSelectorOpIn, fmt.Sprint(cpu(sku).Value())),
		scheduling.NewRequirement(v1alpha1.LabelSKUMemory, v1.NodeSelectorOpIn, fmt.Sprint(memory(sku).ScaledValue(resource.Mega))),

		scheduling.NewRequirement(v1alpha1.LabelSKUTier, v1.NodeSelectorOpDoesNotExist),
		//scheduling.NewRequirement(LabelSKUSizeGen, v1.NodeSelectorOpDoesNotExist),

		// composites
		scheduling.NewRequirement(v1alpha1.LabelSKUName, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUSize, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUSeries, v1.NodeSelectorOpDoesNotExist),

		// size parts
		scheduling.NewRequirement(v1alpha1.LabelSKUFamily, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUSubfamily, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUCPUConstrained, v1.NodeSelectorOpDoesNotExist), // TODO: check predicate specified
		scheduling.NewRequirement(v1alpha1.LabelSKUAccelerator, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUVersion, v1.NodeSelectorOpDoesNotExist),

		// SKU capabilities
		scheduling.NewRequirement(v1alpha1.LabelSKUStoragePremiumCapable, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUEncryptionAtHostSupported, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUEphemeralOSDiskSupported, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUAcceleratedNetworking, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUHyperVGeneration, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUCachedDiskSize, v1.NodeSelectorOpDoesNotExist),
		scheduling.NewRequirement(v1alpha1.LabelSKUMaxResourceVolume, v1.NodeSelectorOpDoesNotExist),

		// all additive feature initialized elsewhere
	)

	// composites
	requirements[v1alpha1.LabelSKUName].Insert(sku.GetName())
	requirements[v1alpha1.LabelSKUSize].Insert(*sku.Size)

	vmsize, err := getVMSize(*sku.Size)
	if err != nil {
		if _, ok := ignoredErrorSKUs[*sku.Size]; !ok {
			//TODO: uncomment after improving parsing
			logging.FromContext(ctx).Errorf("parsing VM size %s, %v", *sku.Size, err)
		}
		return requirements
	}
	// logging.FromContext(ctx).Debugf("VM Size %s: %s", *i.Size, vmsize)

	requirements[v1alpha1.LabelSKUSeries].Insert(vmsize.getSeries())

	// size parts
	requirements[v1alpha1.LabelSKUFamily].Insert(vmsize.family)
	if vmsize.subfamily != nil {
		requirements[v1alpha1.LabelSKUSubfamily].Insert(*vmsize.subfamily)
	}

	if vmsize.cpusConstrained != nil {
		requirements[v1alpha1.LabelSKUCPUConstrained].Insert(*vmsize.cpusConstrained)
	}

	// everything from additive features
	for _, featureLabel := range v1alpha1.SkuFeatureToLabel {
		requirements.Add(scheduling.NewRequirement(featureLabel, v1.NodeSelectorOpDoesNotExist))
	}
	for _, feature := range vmsize.additiveFeatures {
		if featureLabel, ok := v1alpha1.SkuFeatureToLabel[feature]; ok {
			requirements[featureLabel].Insert("true") // TODO: correct way to deal with bool in requirements?
		} else {
			if feature != 'p' && feature != 'r' { // known not in mapping
				logging.FromContext(ctx).Debugf("Ignoring unrecognized feature of VM Size %s: %s", sku.GetName(), string(feature))
			}
		}
	}

	// TODO: Handle zonal availability (IsUltraSSDAvailableInAvailabilityZone).
	// (How? Would have to introduce requirements at offerring level ...)
	if IsPremiumIO(sku) {
		requirements[v1alpha1.LabelSKUStoragePremiumCapable].Insert("true")
	}
	if sku.IsEncryptionAtHostSupported() {
		requirements[v1alpha1.LabelSKUEncryptionAtHostSupported].Insert("true")
	}
	if sku.IsEphemeralOSDiskSupported() && vmsize.getSeries() != "Dlds_v5" { // Dlds_v5 does not support ephemeral OS disk, contrary to what it claims
		requirements[v1alpha1.LabelSKUEphemeralOSDiskSupported].Insert("true")
	}
	if sku.IsAcceleratedNetworkingSupported() {
		requirements[v1alpha1.LabelSKUAcceleratedNetworking].Insert("true") // TODO: correct way to deal with bool in requirements?
	}
	// multiple values for instance type requirement:
	if IsHyperVGen1Supported(sku) {
		requirements[v1alpha1.LabelSKUHyperVGeneration].Insert("V1")
	}
	if sku.IsHyperVGen2Supported() {
		requirements[v1alpha1.LabelSKUHyperVGeneration].Insert("V2")
	}

	if maxCached, err := sku.MaxCachedDiskBytes(); err == nil {
		requirements[v1alpha1.LabelSKUCachedDiskSize].Insert(fmt.Sprint(maxCached))
	}
	if maxTemp, err := sku.MaxResourceVolumeMB(); err == nil {
		requirements[v1alpha1.LabelSKUMaxResourceVolume].Insert(fmt.Sprint(maxTemp))
	}

	if vmsize.acceleratorType != nil {
		requirements[v1alpha1.LabelSKUAccelerator].Insert(*vmsize.acceleratorType)
	}

	requirements[v1alpha1.LabelSKUVersion].Insert(vmsize.version)

	// TODO: more: GPU, etc.

	return requirements
}

func getArchitecture(sku *skewer.SKU) string {
	// TODO: error handling
	architecture, _ := GetCPUArchitectureType(sku)
	if value, ok := v1alpha1.AzureToKubeArchitectures[architecture]; ok {
		return value
	}

	return architecture // unrecognized
}

func computeCapacity(sku *skewer.SKU, kc *v1alpha5.KubeletConfiguration) v1.ResourceList {
	return v1.ResourceList{
		v1.ResourceCPU:              *cpu(sku),
		v1.ResourceMemory:           *memory(sku),
		v1.ResourceEphemeralStorage: *getEphemeralStorage(sku),
		v1.ResourcePods:             *pods(sku, kc),
		// TODO: (important) more: GPU etc.
	}
}

func cpu(sku *skewer.SKU) *resource.Quantity {
	// TODO: error handling
	vcpu, _ := sku.VCPU()
	return resources.Quantity(fmt.Sprint(vcpu))
}

func memory(sku *skewer.SKU) *resource.Quantity {
	// TODO: error handling
	// TODO: Account for VM overhead in calculation
	memory, _ := sku.Memory() // in GB! and float!
	return resources.Quantity(fmt.Sprintf("%fG", memory))
}

// TODO: Don't see a way to get default Azure volume size ...
func getEphemeralStorage(*skewer.SKU) *resource.Quantity {
	return resource.NewScaledQuantity(20, resource.Giga)
}

func pods(sku *skewer.SKU, kc *v1alpha5.KubeletConfiguration) *resource.Quantity {
	// TODO: fine-tune pods calc
	var count int64
	switch {
	case kc != nil && kc.MaxPods != nil:
		count = int64(ptr.Int32Value(kc.MaxPods))
	default:
		count = 110
	}
	// TODO: feature flag for PodsPerCoreEnabled?
	if kc != nil && ptr.Int32Value(kc.PodsPerCore) > 0 {
		count = lo.Min([]int64{int64(ptr.Int32Value(kc.PodsPerCore)) * cpu(sku).Value(), count})
	}
	return resources.Quantity(fmt.Sprint(count))
}

/*
// TODO: no way to distinguish between AMD and Nvidia GPUs
// TODO: skewer should support this natively
func (i *InstanceType) nvidiaGPUs() *resource.Quantity {
	count, err := i.SKU.GetCapabilityIntegerQuantity("GPUs")
	if err != nil {
		count = 0
	}
	return resources.Quantity(fmt.Sprint(count))
}

func (i *InstanceType) amdGPUs() *resource.Quantity {
	count, err := i.SKU.GetCapabilityIntegerQuantity("GPUs")
	if err != nil {
		count = 0
	}
	return resources.Quantity(fmt.Sprint(count))
}
*/

func systemReservedResources(kc *v1alpha5.KubeletConfiguration) v1.ResourceList {
	// default system-reserved resources: https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#system-reserved
	resources := v1.ResourceList{
		v1.ResourceCPU:              resource.MustParse("100m"),
		v1.ResourceMemory:           resource.MustParse("100Mi"),
		v1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
	}
	if kc != nil && kc.SystemReserved != nil {
		return lo.Assign(resources, kc.SystemReserved)
	}
	return resources
}

func kubeReservedResources(cpus, pods *resource.Quantity, kc *v1alpha5.KubeletConfiguration) v1.ResourceList {
	// TODO: replace with Azure/AKS computation; current values and computation are just placeholders, borrowed from AWS provider

	resources := v1.ResourceList{
		v1.ResourceMemory:           resource.MustParse(fmt.Sprintf("%dMi", (11*pods.Value())+255)),
		v1.ResourceEphemeralStorage: resource.MustParse("1Gi"), // default kube-reserved ephemeral-storage
	}

	// kube-reserved Computed from
	// https://github.com/bottlerocket-os/bottlerocket/pull/1388/files#diff-bba9e4e3e46203be2b12f22e0d654ebd270f0b478dd34f40c31d7aa695620f2fR611
	for _, cpuRange := range []struct {
		start      int64
		end        int64
		percentage float64
	}{
		{start: 0, end: 1000, percentage: 0.06},
		{start: 1000, end: 2000, percentage: 0.01},
		{start: 2000, end: 4000, percentage: 0.005},
		{start: 4000, end: 1 << 31, percentage: 0.0025},
	} {
		cpuSt := cpus
		if cpu := cpuSt.MilliValue(); cpu >= cpuRange.start {
			r := float64(cpuRange.end - cpuRange.start)
			if cpu < cpuRange.end {
				r = float64(cpu - cpuRange.start)
			}
			cpuOverhead := resources.Cpu()
			cpuOverhead.Add(*resource.NewMilliQuantity(int64(r*cpuRange.percentage), resource.DecimalSI))
			resources[v1.ResourceCPU] = *cpuOverhead
		}
	}
	if kc != nil && kc.KubeReserved != nil {
		return lo.Assign(resources, kc.KubeReserved)
	}
	return resources
}

func evictionThreshold(memory *resource.Quantity, kc *v1alpha5.KubeletConfiguration) v1.ResourceList {
	overhead := v1.ResourceList{
		v1.ResourceMemory: resource.MustParse("100Mi"),
	}
	if kc == nil {
		return overhead
	}

	override := v1.ResourceList{}
	var evictionSignals []map[string]string
	if kc.EvictionHard != nil {
		evictionSignals = append(evictionSignals, kc.EvictionHard)
	}
	// TODO: feature flag for enabling soft eviction?
	if kc.EvictionSoft != nil {
		evictionSignals = append(evictionSignals, kc.EvictionSoft)
	}

	for _, m := range evictionSignals {
		temp := v1.ResourceList{}
		if v, ok := m[memoryAvailable]; ok {
			if strings.HasSuffix(v, "%") {
				p := mustParsePercentage(v)

				// Calculation is node.capacity * evictionHard[memory.available] if percentage
				// From https://kubernetes.io/docs/concepts/scheduling-eviction/node-pressure-eviction/#eviction-signals
				temp[v1.ResourceMemory] = resource.MustParse(fmt.Sprint(math.Ceil(float64(memory.Value()) / 100 * p)))
			} else {
				temp[v1.ResourceMemory] = resource.MustParse(v)
			}
		}
		override = resources.MaxResources(override, temp)
	}
	// Assign merges maps from left to right so overrides will always be taken last
	return lo.Assign(overhead, override)
}

func mustParsePercentage(v string) float64 {
	p, err := strconv.ParseFloat(strings.Trim(v, "%"), 64)
	if err != nil {
		panic(fmt.Sprintf("expected percentage value to be a float but got %s, %v", v, err))
	}
	// Setting percentage value to 100% is considered disabling the threshold according to
	// https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/
	if p == 100 {
		p = 0
	}
	return p
}
