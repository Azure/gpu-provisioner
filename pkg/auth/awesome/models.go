package awesome

// RotateCertProfile - [Internal Only] The rotate cert profile
type RotateCertProfile struct {
	// The rotate cert mode.
	RotateCertMode *RotateCertMode
}

// RotateCertMode - The rotate cert mode.
type RotateCertMode string

// GPUInstanceProfile - GPUInstanceProfile to be used to specify GPU MIG instance profile for supported GPU VM SKU.
type GPUInstanceProfile string

// KubeletDiskType - Determines the placement of emptyDir volumes, container runtime data root, and Kubelet ephemeral storage.
type KubeletDiskType string

// AgentPoolMode - A cluster must have at least one 'System' Agent Pool at all times. For additional information on agent
// pool restrictions and best practices, see: https://docs.microsoft.com/azure/aks/use-system-pools
type AgentPoolMode string

// OSDiskType - The default is 'Ephemeral' if the VM supports it and has a cache disk larger than the requested OSDiskSizeGB.
// Otherwise, defaults to 'Managed'. May not be changed after creation. For more information
// see Ephemeral OS [https://docs.microsoft.com/azure/aks/cluster-configuration#ephemeral-os].
type OSDiskType string

// OSSKU - Specifies the OS SKU used by the agent pool. If not specified, the default is Ubuntu if OSType=Linux or Windows2019
// if OSType=Windows. And the default Windows OSSKU will be changed to Windows2022
// after Windows2019 is deprecated.
type OSSKU string

// OSType - The operating system type. The default is Linux.
type OSType string

// PodIPAllocationMode - The IP allocation mode for pods in the agent pool. Must be used with podSubnetId. The default is
// 'DynamicIndividual'.
type PodIPAllocationMode string

// ManagedClusterAgentPoolProfileProperties - Properties for the container service agent pool profile.
type ManagedClusterAgentPoolProfileProperties struct {
	// The list of Availability zones to use for nodes. This can only be specified if the AgentPoolType property is 'VirtualMachineScaleSets'.
	AvailabilityZones []*string

	// Number of agents (VMs) to host docker containers. Allowed values must be in the range of 0 to 1000 (inclusive) for user
	// pools and in the range of 1 to 1000 (inclusive) for system pools. The default
	// value is 1.
	Count *int32

	// CreationData to be used to specify the source Snapshot ID if the node pool will be created/upgraded using a snapshot.
	CreationData *CreationData

	// Whether to enable auto-scaler
	EnableAutoScaling *bool

	// When set to true, AKS adds a label to the node indicating that the feature is enabled and deploys a daemonset along with
	// host services to sync custom certificate authorities from user-provided list of
	// base64 encoded certificates into node trust stores. Defaults to false.
	EnableCustomCATrust *bool

	// This is only supported on certain VM sizes and in certain Azure regions. For more information, see: https://docs.microsoft.com/azure/aks/enable-host-encryption
	EnableEncryptionAtHost *bool

	// See Add a FIPS-enabled node pool [https://docs.microsoft.com/azure/aks/use-multiple-node-pools#add-a-fips-enabled-node-pool-preview]
	// for more details.
	EnableFIPS *bool

	// Some scenarios may require nodes in a node pool to receive their own dedicated public IP addresses. A common scenario is
	// for gaming workloads, where a console needs to make a direct connection to a
	// cloud virtual machine to minimize hops. For more information see assigning a public IP per node
	// [https://docs.microsoft.com/azure/aks/use-multiple-node-pools#assign-a-public-ip-per-node-for-your-node-pools]. The default
	// is false.
	EnableNodePublicIP *bool

	// Whether to enable UltraSSD
	EnableUltraSSD *bool

	// GPUInstanceProfile to be used to specify GPU MIG instance profile for supported GPU VM SKU.
	GpuInstanceProfile *GPUInstanceProfile

	// This is of the form: /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/hostGroups/{hostGroupName}.
	// For more information see Azure dedicated hosts
	// [https://docs.microsoft.com/azure/virtual-machines/dedicated-hosts].
	HostGroupID *string

	// The Kubelet configuration on the agent pool nodes.
	KubeletConfig *KubeletConfig

	// Determines the placement of emptyDir volumes, container runtime data root, and Kubelet ephemeral storage.
	KubeletDiskType *KubeletDiskType

	// The OS configuration of Linux agent nodes.
	LinuxOSConfig *LinuxOSConfig

	// The maximum number of nodes for auto-scaling
	MaxCount *int32

	// The maximum number of pods that can run on a node.
	MaxPods *int32

	// The minimum number of nodes for auto-scaling
	MinCount *int32

	// A cluster must have at least one 'System' Agent Pool at all times. For additional information on agent pool restrictions
	// and best practices, see: https://docs.microsoft.com/azure/aks/use-system-pools
	Mode *AgentPoolMode

	// Network-related settings of an agent pool.
	NetworkProfile *AgentPoolNetworkProfile

	// The node labels to be persisted across all nodes in agent pool.
	NodeLabels map[string]*string

	// This is of the form: /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/publicIPPrefixes/{publicIPPrefixName}
	NodePublicIPPrefixID *string

	// The taints added to new nodes during node pool create and scale. For example, key=value:NoSchedule.
	NodeTaints []*string

	// OS Disk Size in GB to be used to specify the disk size for every machine in the master/agent pool. If you specify 0, it
	// will apply the default osDisk size according to the vmSize specified.
	OSDiskSizeGB *int32

	// The default is 'Ephemeral' if the VM supports it and has a cache disk larger than the requested OSDiskSizeGB. Otherwise,
	// defaults to 'Managed'. May not be changed after creation. For more information
	// see Ephemeral OS [https://docs.microsoft.com/azure/aks/cluster-configuration#ephemeral-os].
	OSDiskType *OSDiskType

	// Specifies the OS SKU used by the agent pool. If not specified, the default is Ubuntu if OSType=Linux or Windows2019 if
	// OSType=Windows. And the default Windows OSSKU will be changed to Windows2022
	// after Windows2019 is deprecated.
	OSSKU *OSSKU

	// The operating system type. The default is Linux.
	OSType *OSType

	// Both patch version and are supported. When is specified, the latest supported patch version is chosen automatically. Updating
	// the agent pool with the same once it has been created will not trigger an
	// upgrade, even if a newer patch version is available. As a best practice, you should upgrade all node pools in an AKS cluster
	// to the same Kubernetes version. The node pool version must have the same
	// major version as the control plane. The node pool minor version must be within two minor versions of the control plane
	// version. The node pool version cannot be greater than the control plane version.
	// For more information see upgrading a node pool [https://docs.microsoft.com/azure/aks/use-multiple-node-pools#upgrade-a-node-pool].
	OrchestratorVersion *string

	// If omitted, pod IPs are statically assigned on the node subnet (see vnetSubnetID for more details). This is of the form:
	// /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/virtualNetworks/{virtualNetworkName}/subnets/{subnetName}
	PodSubnetID *string

	// When an Agent Pool is first created it is initially Running. The Agent Pool can be stopped by setting this field to Stopped.
	// A stopped Agent Pool stops all of its VMs and does not accrue billing
	// charges. An Agent Pool can only be stopped if it is Running and provisioning state is Succeeded
	PowerState *PowerState

	// The ID for Proximity Placement Group.
	ProximityPlacementGroupID *string

	// This also effects the cluster autoscaler behavior. If not specified, it defaults to Delete.
	ScaleDownMode *ScaleDownMode

	// This cannot be specified unless the scaleSetPriority is 'Spot'. If not specified, the default is 'Delete'.
	ScaleSetEvictionPolicy *ScaleSetEvictionPolicy

	// The Virtual Machine Scale Set priority. If not specified, the default is 'Regular'.
	ScaleSetPriority *ScaleSetPriority

	// Possible values are any decimal value greater than zero or -1 which indicates the willingness to pay any on-demand price.
	// For more details on spot pricing, see spot VMs pricing
	// [https://docs.microsoft.com/azure/virtual-machines/spot-vms#pricing]
	SpotMaxPrice *float32

	// The tags to be persisted on the agent pool virtual machine scale set.
	Tags map[string]*string

	// The type of Agent Pool.
	Type *AgentPoolType

	// Settings for upgrading the agentpool
	UpgradeSettings *AgentPoolUpgradeSettings

	// VM size availability varies by region. If a node contains insufficient compute resources (memory, cpu, etc) pods might
	// fail to run correctly. For more details on restricted VM sizes, see:
	// https://docs.microsoft.com/azure/aks/quotas-skus-regions
	VMSize *string

	// If this is not specified, a VNET and subnet will be generated and used. If no podSubnetID is specified, this applies to
	// nodes and pods, otherwise it applies to just nodes. This is of the form:
	// /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Network/virtualNetworks/{virtualNetworkName}/subnets/{subnetName}
	VnetSubnetID *string

	// The Windows agent pool's specific profile.
	WindowsProfile *AgentPoolWindowsProfile

	// Determines the type of workload a node can run.
	WorkloadRuntime *WorkloadRuntime

	// READ-ONLY; If orchestratorVersion was a fully specified version , this field will be exactly equal to it. If orchestratorVersion
	// was , this field will contain the full version being used.
	CurrentOrchestratorVersion *string

	// READ-ONLY; Unique read-only string used to implement optimistic concurrency. The eTag value will change when the resource
	// is updated. Specify an if-match or if-none-match header with the eTag value for a
	// subsequent request to enable optimistic concurrency per the normal etag convention.
	ETag *string

	// READ-ONLY; The version of node image
	NodeImageVersion *string

	// READ-ONLY; The current deployment or provisioning state.
	ProvisioningState *string
}

// ScaleProfile - Specifications on how to scale a VirtualMachines agent pool.
type ScaleProfile struct {
	// Specifications on how to auto-scale the VirtualMachines agent pool within a predefined size range. Currently, at most one
	// AutoScaleProfile is allowed.
	Autoscale []*AutoScaleProfile

	// Specifications on how to scale the VirtualMachines agent pool to a fixed size. Currently, at most one ManualScaleProfile
	// is allowed.
	Manual []*ManualScaleProfile
}

// ManualScaleProfile - Specifications on number of machines.
type ManualScaleProfile struct {
	// Number of nodes.
	Count *int32

	// The list of allowed vm sizes e.g. ['StandardE4sv3', 'StandardE16sv3', 'StandardD16sv5']. AKS will use the first available
	// one when scaling. If a VM size is unavailable (e.g. due to quota or regional
	// capacity reasons), AKS will use the next size.
	Sizes []*string
}

// AgentPoolType - The type of Agent Pool.
type AgentPoolType string

// AutoScaleProfile - Specifications on auto-scaling.
type AutoScaleProfile struct {
	// The maximum number of nodes of the specified sizes.
	MaxCount *int32

	// The minimum number of nodes of the specified sizes.
	MinCount *int32

	// The list of allowed vm sizes e.g. ['StandardE4sv3', 'StandardE16sv3', 'StandardD16sv5']. AKS will use the first available
	// one when auto scaling. If a VM size is unavailable (e.g. due to quota or
	// regional capacity reasons), AKS will use the next size.
	Sizes []*string
}

// VirtualMachinesProfile - Specifications on VirtualMachines agent pool.
type VirtualMachinesProfile struct {
	// Specifications on how to scale a VirtualMachines agent pool.
	Scale *ScaleProfile
}

// WorkloadRuntime - Determines the type of workload a node can run.
type WorkloadRuntime string

type AgentPoolArtifactStreamingProfile struct {
	// Artifact streaming speeds up the cold-start of containers on a node through on-demand image loading. To use this feature,
	// container images must also enable artifact streaming on ACR. If not specified,
	// the default is false.
	Enabled *bool
}

// VirtualMachineNodes - Current status on a group of nodes of the same vm size.
type VirtualMachineNodes struct {
	// Number of nodes.
	Count *int32

	// The VM size of the agents used to host this group of nodes.
	Size *string
}

// CreationData - Data used when creating a target resource from a source resource.
type CreationData struct {
	// This is the ARM ID of the source object to be used to create the target object.
	SourceResourceID *string
}

// KubeletConfig - See AKS custom node configuration [https://docs.microsoft.com/azure/aks/custom-node-configuration] for
// more details.
type KubeletConfig struct {
	// Allowed list of unsafe sysctls or unsafe sysctl patterns (ending in *).
	AllowedUnsafeSysctls []*string

	// The default is true.
	CPUCfsQuota *bool

	// The default is '100ms.' Valid values are a sequence of decimal numbers with an optional fraction and a unit suffix. For
	// example: '300ms', '2h45m'. Supported units are 'ns', 'us', 'ms', 's', 'm', and
	// 'h'.
	CPUCfsQuotaPeriod *string

	// The default is 'none'. See Kubernetes CPU management policies [https://kubernetes.io/docs/tasks/administer-cluster/cpu-management-policies/#cpu-management-policies]
	// for more information. Allowed
	// values are 'none' and 'static'.
	CPUManagerPolicy *string

	// The maximum number of container log files that can be present for a container. The number must be ≥ 2.
	ContainerLogMaxFiles *int32

	// The maximum size (e.g. 10Mi) of container log file before it is rotated.
	ContainerLogMaxSizeMB *int32

	// If set to true it will make the Kubelet fail to start if swap is enabled on the node.
	FailSwapOn *bool

	// To disable image garbage collection, set to 100. The default is 85%
	ImageGcHighThreshold *int32

	// This cannot be set higher than imageGcHighThreshold. The default is 80%
	ImageGcLowThreshold *int32

	// The maximum number of processes per pod.
	PodMaxPids *int32

	// For more information see Kubernetes Topology Manager [https://kubernetes.io/docs/tasks/administer-cluster/topology-manager].
	// The default is 'none'. Allowed values are 'none', 'best-effort',
	// 'restricted', and 'single-numa-node'.
	TopologyManagerPolicy *string
}

// LinuxOSConfig - See AKS custom node configuration [https://docs.microsoft.com/azure/aks/custom-node-configuration] for
// more details.
type LinuxOSConfig struct {
	// The size in MB of a swap file that will be created on each node.
	SwapFileSizeMB *int32

	// Sysctl settings for Linux agent nodes.
	Sysctls *SysctlConfig

	// Valid values are 'always', 'defer', 'defer+madvise', 'madvise' and 'never'. The default is 'madvise'. For more information
	// see Transparent Hugepages
	// [https://www.kernel.org/doc/html/latest/admin-guide/mm/transhuge.html#admin-guide-transhuge].
	TransparentHugePageDefrag *string

	// Valid values are 'always', 'madvise', and 'never'. The default is 'always'. For more information see Transparent Hugepages
	// [https://www.kernel.org/doc/html/latest/admin-guide/mm/transhuge.html#admin-guide-transhuge].
	TransparentHugePageEnabled *string

	// Ulimit settings for Linux agent nodes.
	Ulimits *UlimitConfig
}

// UlimitConfig - Ulimit settings for Linux agent nodes
type UlimitConfig struct {
	// Maximum locked-in-memory address space (KB)
	MaxLockedMemory *string

	// Maximum number of open files
	NoFile *string
}

// SysctlConfig - Sysctl settings for Linux agent nodes.
type SysctlConfig struct {
	// Sysctl setting fs.aio-max-nr.
	FsAioMaxNr *int32

	// Sysctl setting fs.file-max.
	FsFileMax *int32

	// Sysctl setting fs.inotify.maxuserwatches.
	FsInotifyMaxUserWatches *int32

	// Sysctl setting fs.nr_open.
	FsNrOpen *int32

	// Sysctl setting kernel.threads-max.
	KernelThreadsMax *int32

	// Sysctl setting net.core.netdevmaxbacklog.
	NetCoreNetdevMaxBacklog *int32

	// Sysctl setting net.core.optmem_max.
	NetCoreOptmemMax *int32

	// Sysctl setting net.core.rmem_default.
	NetCoreRmemDefault *int32

	// Sysctl setting net.core.rmem_max.
	NetCoreRmemMax *int32

	// Sysctl setting net.core.somaxconn.
	NetCoreSomaxconn *int32

	// Sysctl setting net.core.wmem_default.
	NetCoreWmemDefault *int32

	// Sysctl setting net.core.wmem_max.
	NetCoreWmemMax *int32

	// Sysctl setting net.ipv4.iplocalport_range.
	NetIPv4IPLocalPortRange *string

	// Sysctl setting net.ipv4.neigh.default.gc_thresh1.
	NetIPv4NeighDefaultGcThresh1 *int32

	// Sysctl setting net.ipv4.neigh.default.gc_thresh2.
	NetIPv4NeighDefaultGcThresh2 *int32

	// Sysctl setting net.ipv4.neigh.default.gc_thresh3.
	NetIPv4NeighDefaultGcThresh3 *int32

	// Sysctl setting net.ipv4.tcpfintimeout.
	NetIPv4TCPFinTimeout *int32

	// Sysctl setting net.ipv4.tcpkeepaliveprobes.
	NetIPv4TCPKeepaliveProbes *int32

	// Sysctl setting net.ipv4.tcpkeepalivetime.
	NetIPv4TCPKeepaliveTime *int32

	// Sysctl setting net.ipv4.tcpmaxsyn_backlog.
	NetIPv4TCPMaxSynBacklog *int32

	// Sysctl setting net.ipv4.tcpmaxtw_buckets.
	NetIPv4TCPMaxTwBuckets *int32

	// Sysctl setting net.ipv4.tcptwreuse.
	NetIPv4TCPTwReuse *bool

	// Sysctl setting net.ipv4.tcpkeepaliveintvl.
	NetIPv4TcpkeepaliveIntvl *int32

	// Sysctl setting net.netfilter.nfconntrackbuckets.
	NetNetfilterNfConntrackBuckets *int32

	// Sysctl setting net.netfilter.nfconntrackmax.
	NetNetfilterNfConntrackMax *int32

	// Sysctl setting vm.maxmapcount.
	VMMaxMapCount *int32

	// Sysctl setting vm.swappiness.
	VMSwappiness *int32

	// Sysctl setting vm.vfscachepressure.
	VMVfsCachePressure *int32
}

// PowerState - Describes the Power State of the cluster
type PowerState struct {
	// Tells whether the cluster is Running or Stopped
	Code *Code
}

// Code - Tells whether the cluster is Running or Stopped
type Code string

// ScaleDownMode - Describes how VMs are added to or removed from Agent Pools. See billing states [https://docs.microsoft.com/azure/virtual-machines/states-billing].
type ScaleDownMode string

// ScaleSetEvictionPolicy - The eviction policy specifies what to do with the VM when it is evicted. The default is Delete.
// For more information about eviction see spot VMs
// [https://docs.microsoft.com/azure/virtual-machines/spot-vms]
type ScaleSetEvictionPolicy string

// ScaleSetPriority - The Virtual Machine Scale Set priority.
type ScaleSetPriority string

// IPTag - Contains the IPTag associated with the object.
type IPTag struct {
	// The IP tag type. Example: RoutingPreference.
	IPTagType *string

	// The value of the IP tag associated with the public IP. Example: Internet.
	Tag *string
}

// AgentPoolSSHAccess - SSH access method of an agent pool.
type AgentPoolSSHAccess string

// PortRange - The port range.
type PortRange struct {
	// The maximum port that is included in the range. It should be ranged from 1 to 65535, and be greater than or equal to portStart.
	PortEnd *int32

	// The minimum port that is included in the range. It should be ranged from 1 to 65535, and be less than or equal to portEnd.
	PortStart *int32

	// The network protocol of the port.
	Protocol *Protocol
}

// AgentPoolUpgradeProfile - The list of available upgrades for an agent pool.
type AgentPoolUpgradeProfile struct {
	// REQUIRED; The properties of the agent pool upgrade profile.
	Properties *AgentPoolUpgradeProfileProperties

	// READ-ONLY; The ID of the agent pool upgrade profile.
	ID *string

	// READ-ONLY; The name of the agent pool upgrade profile.
	Name *string

	// READ-ONLY; The type of the agent pool upgrade profile.
	Type *string
}

// AgentPoolUpgradeProfileProperties - The list of available upgrade versions.
type AgentPoolUpgradeProfileProperties struct {
	// REQUIRED; The Kubernetes version (major.minor.patch).
	KubernetesVersion *string

	// REQUIRED; The operating system type. The default is Linux.
	OSType *OSType

	// components of given Kubernetes version.
	ComponentsByReleases *ComponentsByReleases

	// The latest AKS supported node image version.
	LatestNodeImageVersion *string

	// List of orchestrator types and versions available for upgrade.
	Upgrades []*AgentPoolUpgradeProfilePropertiesUpgradesItem
}

type AgentPoolUpgradeProfilePropertiesUpgradesItem struct {
	// Whether the Kubernetes version is currently in preview.
	IsPreview *bool

	// The Kubernetes version (major.minor.patch).
	KubernetesVersion *string
}

// AgentPoolUpgradeSettings - Settings for upgrading an agentpool
type AgentPoolUpgradeSettings struct {
	// The amount of time (in minutes) to wait on eviction of pods and graceful termination per node. This eviction wait time
	// honors waiting on pod disruption budgets. If this time is exceeded, the upgrade
	// fails. If not specified, the default is 30 minutes.
	DrainTimeoutInMinutes *int32

	// This can either be set to an integer (e.g. '5') or a percentage (e.g. '50%'). If a percentage is specified, it is the percentage
	// of the total agent pool size at the time of the upgrade. For
	// percentages, fractional nodes are rounded up. If not specified, the default is 1. For more information, including best
	// practices, see:
	// https://docs.microsoft.com/azure/aks/upgrade-cluster#customize-node-surge-upgrade
	MaxSurge *string

	// The amount of time (in minutes) to wait after draining a node and before reimaging it and moving on to next node. If not
	// specified, the default is 0 minutes.
	NodeSoakDurationInMinutes *int32

	// Defines the behavior for undrainable nodes during upgrade. The most common cause of undrainable nodes is Pod Disruption
	// Budgets (PDBs), but other issues, such as pod termination grace period is
	// exceeding the remaining per-node drain timeout or pod is still being in a running state, can also cause undrainable nodes.
	UndrainableNodeBehavior *UndrainableNodeBehavior
}

// AgentPoolWindowsProfile - The Windows agent pool's specific profile.
type AgentPoolWindowsProfile struct {
	// The default value is false. Outbound NAT can only be disabled if the cluster outboundType is NAT Gateway and the Windows
	// agent pool does not have node public IP enabled.
	DisableOutboundNat *bool
}

type Component struct {
	// If upgraded component version contains breaking changes from the current version. To see a detailed description of what
	// the breaking changes are, visit
	// https://learn.microsoft.com/azure/aks/supported-kubernetes-versions?tabs=azure-cli#aks-components-breaking-changes-by-version.
	HasBreakingChanges *bool

	// Component name.
	Name *string

	// Component version.
	Version *string
}

// ComponentsByReleases - components of given Kubernetes version.
type ComponentsByReleases struct {
	// components of current or upgraded Kubernetes version in the cluster.
	Components []*Component

	// The Kubernetes version (major.minor).
	KubernetesVersion *string
}

// UndrainableNodeBehavior - Defines the behavior for undrainable nodes during upgrade. The most common cause of undrainable
// nodes is Pod Disruption Budgets (PDBs), but other issues, such as pod termination grace period is
// exceeding the remaining per-node drain timeout or pod is still being in a running state, can also cause undrainable nodes.
type UndrainableNodeBehavior string

// Protocol - The network protocol of the port.
type Protocol string
