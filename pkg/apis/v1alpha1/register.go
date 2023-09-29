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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	//nolint SA1019 - deprecated package
	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
)

const ()

var (
	LabelDomain = "karpenter.k8s.azure"

	// TODO: Double check that we don't need to support "Low"
	// TODO: Consider renaming to PrioritySpot/Regular
	// TODO: Q: These might actually be values gpu-provisioner-core knows about,
	// may not be able to use Regular instead of On-demand
	PrioritySpot             = string(compute.Spot)
	PriorityRegular          = string(compute.Regular)
	AzureToKubeArchitectures = map[string]string{
		// TODO: consider using constants like compute.ArchitectureArm64
		"x64":   v1alpha5.ArchitectureAmd64,
		"Arm64": v1alpha5.ArchitectureArm64,
	}
	RestrictedLabelDomains = []string{
		LabelDomain,
	}

	// alternative zone label for Machine (the standard one is protected for AKS nodes)
	AlternativeLabelTopologyZone = LabelDomain + "/zone"

	ManufacturerNvidia = "nvidia"

	// TODO: this set needs to be designed properly and carefully; essentially represents the API

	LabelSKUTier = LabelDomain + "/sku-tier" // Basic, Standard [, Premium?]

	// composites
	LabelSKUName   = LabelDomain + "/sku-name"   // Standard_A1_v2
	LabelSKUSize   = LabelDomain + "/sku-size"   // A1_v2
	LabelSKUSeries = LabelDomain + "/sku-series" // DCSv3 (Family+Subfamily+AdditionalFeatures+Version]

	// size parts
	LabelSKUFamily         = LabelDomain + "/sku-family"    // standardAv2Family
	LabelSKUSubfamily      = LabelDomain + "/sku-subfamily" // TODO:example
	LabelSKUCPU            = LabelDomain + "/sku-cpu"       // sku.vCPUs
	LabelSKUCPUConstrained = LabelDomain + "/sku-cpu-constrained"
	LabelSKUAccelerator    = LabelDomain + "/sku-accelerator"
	LabelSKUVersion        = LabelDomain + "/sku-version" // v1 (?), v2,v3,...

	// capabilities (from additive features in VM size name, or from SKU capabilities)
	// https://learn.microsoft.com/en-us/azure/virtual-machines/vm-naming-conventions
	LabelSKUCpuTypeAmd              = LabelDomain + "/sku-cpu-type-amd"              // a
	LabelSKUStorageBlockPerformance = LabelDomain + "/sku-storage-block-performance" // b
	LabelSKUConfidential            = LabelDomain + "/sku-confidential"              // c
	LabelSKUStorageDiskful          = LabelDomain + "/sku-storage-diskful"           // d
	LabelSKUIsolatedSize            = LabelDomain + "/sku-isolated-size"             // i
	LabelSKUMemoryLow               = LabelDomain + "/sku-memory-low"                // l
	LabelSKUMemoryIntensive         = LabelDomain + "/sku-memory-intensive"          // m
	LabelSKUMemoryTiny              = LabelDomain + "/sku-memory-tiny"               // t
	LabelSKUStoragePremiumCapable   = LabelDomain + "/sku-storage-premium-capable"   // s = sku.UltraSSDAvailable (?)
	//LabelSKUNodePacking           = LabelDomain + "/sku-node-packing"              // NP TODO: not handled
	//LabelSKUArmCPU                = LabelDomain + "/sku-arm-cpu"                   // P - already covered by architecture label

	LabelSKUMemory                = LabelDomain + "/sku-memory"                 // sku.MemoryGB
	LabelSKUHyperVGeneration      = LabelDomain + "/sku-hyperv-generation"      // sku.HyperVGenerations
	LabelSKUAcceleratedNetworking = LabelDomain + "/sku-networking-accelerated" // sku.AcceleratedNetworkingEnabled

	LabelSKUEncryptionAtHostSupported = LabelDomain + "/sku-encryptionathost-capable"     // sku.EncryptionAtHostSupported
	LabelSKUEphemeralOSDiskSupported  = LabelDomain + "/sku-storage-os-ephemeral-capable" // sku.EphemeralOSDiskSupported
	LabelSKUCachedDiskSize            = LabelDomain + "/sku-storage-cache-maxsize"        // sku.CachedDiskBytes
	LabelSKUMaxResourceVolume         = LabelDomain + "/sku-storage-temp-maxsize"         // sku.MaxResourceVolumeMB
	// TODO: more labels
	// GPU LABELS!
	LabelSKUGPUName         = LabelDomain + "/sku-gpu-name"         // ie GPU Accelerator type we parse from vmSize
	LabelSKUGPUManufacturer = LabelDomain + "/sku-gpu-manufacturer" // ie NVIDIA, AMD, etc
	LabelSKUGPUCount        = LabelDomain + "/sku-gpu-count"        // ie 16, 32, etc

	SkuFeatureToLabel = map[rune]string{
		'a': LabelSKUCpuTypeAmd,
		'b': LabelSKUStorageBlockPerformance,
		'c': LabelSKUConfidential,
		'd': LabelSKUStorageDiskful,
		'i': LabelSKUIsolatedSize,
		'l': LabelSKUMemoryLow,
		'm': LabelSKUMemoryIntensive,
		't': LabelSKUMemoryTiny,
		's': LabelSKUStoragePremiumCapable,
	}
)

var (
	Scheme             = runtime.NewScheme()
	Group              = "karpenter.k8s.azure"
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: "v1alpha1"}
	SchemeBuilder      = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(SchemeGroupVersion)
		metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
		return nil
	})
)

func init() {
	Scheme.AddKnownTypes(schema.GroupVersion{Group: v1alpha5.ExtensionsGroup, Version: "v1alpha1"}, &Azure{})
	v1alpha5.RestrictedLabelDomains = v1alpha5.RestrictedLabelDomains.Insert(RestrictedLabelDomains...)
	v1alpha5.WellKnownLabels = v1alpha5.WellKnownLabels.Insert(
		LabelSKUTier,
		LabelSKUName,
		LabelSKUSize,
		LabelSKUSeries,
		LabelSKUFamily,
		LabelSKUSubfamily,
		LabelSKUCPU,
		LabelSKUCPUConstrained,
		LabelSKUAccelerator,
		LabelSKUVersion,

		LabelSKUCpuTypeAmd,
		LabelSKUStorageBlockPerformance,
		LabelSKUConfidential,
		LabelSKUStorageDiskful,
		LabelSKUIsolatedSize,
		LabelSKUMemoryLow,
		LabelSKUMemoryIntensive,
		LabelSKUMemoryTiny,
		LabelSKUStoragePremiumCapable,

		LabelSKUMemory,
		LabelSKUHyperVGeneration,
		LabelSKUAcceleratedNetworking,

		LabelSKUEncryptionAtHostSupported,
		LabelSKUEphemeralOSDiskSupported,
		LabelSKUCachedDiskSize,
		LabelSKUMaxResourceVolume,

		LabelSKUGPUName,
		LabelSKUGPUManufacturer,
		LabelSKUGPUCount,
	)
}
