package awesome

// AgentPoolsClientAbortLatestOperationResponse contains the response from method AgentPoolsClient.BeginAbortLatestOperation.
type AgentPoolsClientAbortLatestOperationResponse struct {
	// placeholder for future response values
}

// AgentPoolsClientCaptureSecurityVHDSnapshotResponse contains the response from method AgentPoolsClient.BeginCaptureSecurityVHDSnapshot.
type AgentPoolsClientCaptureSecurityVHDSnapshotResponse struct {
	// placeholder for future response values
}

// AgentPoolsClientCreateOrUpdateResponse contains the response from method AgentPoolsClient.BeginCreateOrUpdate.
type AgentPoolsClientCreateOrUpdateResponse struct {
	// Agent Pool.
	AgentPool
}

// AgentPoolsClientDeleteMachinesResponse contains the response from method AgentPoolsClient.BeginDeleteMachines.
type AgentPoolsClientDeleteMachinesResponse struct {
	// placeholder for future response values
}

// AgentPoolsClientDeleteResponse contains the response from method AgentPoolsClient.BeginDelete.
type AgentPoolsClientDeleteResponse struct {
	// placeholder for future response values
}

// AgentPoolsClientGetAvailableAgentPoolVersionsResponse contains the response from method AgentPoolsClient.GetAvailableAgentPoolVersions.
type AgentPoolsClientGetAvailableAgentPoolVersionsResponse struct {
	// The list of available versions for an agent pool.
	AgentPoolAvailableVersions
}

// AgentPoolsClientGetResponse contains the response from method AgentPoolsClient.Get.
type AgentPoolsClientGetResponse struct {
	// Agent Pool.
	AgentPool
}

// AgentPoolsClientGetUpgradeProfileResponse contains the response from method AgentPoolsClient.GetUpgradeProfile.
type AgentPoolsClientGetUpgradeProfileResponse struct {
	// The list of available upgrades for an agent pool.
	AgentPoolUpgradeProfile
}

// AgentPoolsClientListResponse contains the response from method AgentPoolsClient.NewListPager.
type AgentPoolsClientListResponse struct {
	// The response from the List Agent Pools operation.
	AgentPoolListResult
}

// AgentPoolsClientUpgradeNodeImageVersionResponse contains the response from method AgentPoolsClient.BeginUpgradeNodeImageVersion.
type AgentPoolsClientUpgradeNodeImageVersionResponse struct {
	// Agent Pool.
	AgentPool
}

// AgentPool - Agent Pool.
type AgentPool struct {
	// Properties of an agent pool.
	Properties *ManagedClusterAgentPoolProfileProperties

	// READ-ONLY; Resource ID.
	ID *string

	// READ-ONLY; The name of the resource that is unique within a resource group. This name can be used to access the resource.
	Name *string

	// READ-ONLY; Resource type
	Type *string
}

// AgentPoolAvailableVersions - The list of available versions for an agent pool.
type AgentPoolAvailableVersions struct {
	// REQUIRED; Properties of agent pool available versions.
	Properties *AgentPoolAvailableVersionsProperties

	// READ-ONLY; The ID of the agent pool version list.
	ID *string

	// READ-ONLY; The name of the agent pool version list.
	Name *string

	// READ-ONLY; Type of the agent pool version list.
	Type *string
}

// AgentPoolAvailableVersionsProperties - The list of available agent pool versions.
type AgentPoolAvailableVersionsProperties struct {
	// List of versions available for agent pool.
	AgentPoolVersions []*AgentPoolAvailableVersionsPropertiesAgentPoolVersionsItem
}

type AgentPoolAvailableVersionsPropertiesAgentPoolVersionsItem struct {
	// Whether this version is the default agent pool version.
	Default *bool

	// Whether Kubernetes version is currently in preview.
	IsPreview *bool

	// The Kubernetes version (major.minor.patch).
	KubernetesVersion *string
}

// AgentPoolDeleteMachinesParameter - Specifies a list of machine names from the agent pool to be deleted.
type AgentPoolDeleteMachinesParameter struct {
	// REQUIRED; The agent pool machine names.
	MachineNames []*string
}

type AgentPoolGPUProfile struct {
	// The default value is true when the vmSize of the agent pool contains a GPU, false otherwise. GPU Driver Installation can
	// only be set true when VM has an associated GPU resource. Setting this field to
	// false prevents automatic GPU driver installation. In that case, in order for the GPU to be usable, the user must perform
	// GPU driver installation themselves.
	InstallGPUDriver *bool
}

// AgentPoolGatewayProfile - The profile of a gateway agent pool.
type AgentPoolGatewayProfile struct {
	// The size of Public IPv4 Prefix supported by the gateway agent pool. All prefixes applied to the gateway agent pool must
	// be of this size. This provides the upper limit of node count in the gateway
	// agent pool. Value value range is [28, 31]. The default value is 31.
	PublicIPPrefixSize *int32
}

// AgentPoolListResult - The response from the List Agent Pools operation.
type AgentPoolListResult struct {
	// The list of agent pools.
	Value []*AgentPool

	// READ-ONLY; The URL to get the next set of agent pool results.
	NextLink *string
}

// AgentPoolNetworkProfile - Network settings of an agent pool.
type AgentPoolNetworkProfile struct {
	// The port ranges that are allowed to access. The specified ranges are allowed to overlap.
	AllowedHostPorts []*PortRange

	// The IDs of the application security groups which agent pool will associate when created.
	ApplicationSecurityGroups []*string

	// IPTags of instance-level public IPs.
	NodePublicIPTags []*IPTag
}

// AgentPoolSecurityProfile - The security settings of an agent pool.
type AgentPoolSecurityProfile struct {
	// Secure Boot is a feature of Trusted Launch which ensures that only signed operating systems and drivers can boot. For more
	// details, see aka.ms/aks/trustedlaunch. If not specified, the default is
	// false.
	EnableSecureBoot *bool

	// vTPM is a Trusted Launch feature for configuring a dedicated secure vault for keys and measurements held locally on the
	// node. For more details, see aka.ms/aks/trustedlaunch. If not specified, the
	// default is false.
	EnableVTPM *bool

	// SSH access method of an agent pool.
	SSHAccess *AgentPoolSSHAccess
}
