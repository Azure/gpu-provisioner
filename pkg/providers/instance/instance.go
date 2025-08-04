/*
       Copyright (c) Microsoft Corporation.
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

package instance

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/azure/gpu-provisioner/pkg/utils"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

const (
	LabelMachineType       = "kaito.sh/machine-type"
	NodeClaimCreationLabel = "kaito.sh/creation-timestamp"
	// use self-defined layout in order to satisfy node label syntax
	CreationTimestampLayout = "2006-01-02T15-04-05Z"
)

var (
	KaitoNodeLabels    = []string{"kaito.sh/workspace", "kaito.sh/ragengine"}
	AgentPoolNameRegex = regexp.MustCompile(`^[a-z][a-z0-9]{0,11}$`)
)

type Provider struct {
	azClient       interface{} // Can be *AZClient or *ArcAZClient
	kubeClient     client.Client
	resourceGroup  string
	clusterName    string
	subscriptionID string
}

func NewProvider(
	azClient interface{},
	kubeClient client.Client,
	resourceGroup string,
	clusterName string,
	subscriptionID string,
) *Provider {
	return &Provider{
		azClient:       azClient,
		kubeClient:     kubeClient,
		resourceGroup:  resourceGroup,
		clusterName:    clusterName,
		subscriptionID: subscriptionID,
	}
}

// buildAgentPoolParams creates AgentPoolParams for the interface calls
func (p *Provider) buildAgentPoolParams(agentPoolName string, agentPoolSpec interface{}) AgentPoolParams {
	return AgentPoolParams{
		SubscriptionID: p.subscriptionID,
		ResourceGroup:  p.resourceGroup,
		ClusterName:    p.clusterName,
		AgentPoolName:  agentPoolName,
		AgentPoolSpec:  agentPoolSpec,
	}
}

// Create an instance given the constraints.
// instanceTypes should be sorted by priority for spot capacity type.
func (p *Provider) Create(ctx context.Context, nodeClaim *karpenterv1.NodeClaim) (*Instance, error) {
	klog.InfoS("Instance.Create", "nodeClaim", klog.KObj(nodeClaim))

	// We made a strong assumption here. The nodeClaim name should be a valid agent pool name without "-".
	apName := nodeClaim.Name
	if !AgentPoolNameRegex.MatchString(apName) {
		//https://learn.microsoft.com/en-us/troubleshoot/azure/azure-kubernetes/aks-common-issues-faq#what-naming-restrictions-are-enforced-for-aks-resources-and-parameters-
		return nil, fmt.Errorf("agentpool name(%s) is invalid, must match regex pattern: ^[a-z][a-z0-9]{0,11}$", apName)
	}

	var instance *Instance
	err := retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return false
	}, func() error {
		instanceTypes := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaim.Spec.Requirements...).Get("node.kubernetes.io/instance-type").Values()
		if len(instanceTypes) == 0 {
			return fmt.Errorf("nodeClaim spec has no requirement for instance type")
		}

		vmSize := instanceTypes[0]
		apObj, apErr := newAgentPoolObject(vmSize, nodeClaim)
		if apErr != nil {
			return apErr
		}

		logging.FromContext(ctx).Debugf("creating Agent pool %s (%s)", apName, vmSize)

		agentPoolClient := p.extractAgentPoolClient()
		if agentPoolClient == nil {
			return fmt.Errorf("unsupported client type")
		}

		params := p.buildAgentPoolParams(apName, apObj)
		agentPoolInfo, err := agentPoolClient.CreateOrUpdate(ctx, params)
		if err != nil {
			switch {
			case strings.Contains(err.Error(), "Operation is not allowed because there's an in progress create node pool operation"):
				// when gpu-provisioner restarted after crash for unknown reason, we may come across this error that agent pool creating
				// is in progress, so we just need to wait node ready based on the apObj.
				// Create a temporary instance from the original spec
				instance = p.createInstanceFromAgentPoolObject(apObj, apName)
				return nil
			default:
				logging.FromContext(ctx).Errorf("failed to create agent pool for nodeclaim(%s), %v", nodeClaim.Name, err)
				return fmt.Errorf("agentPool.BeginCreateOrUpdate for %q failed: %w", apName, err)
			}
		}

		// Create instance ID
		instanceID := fmt.Sprintf("azure:///subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s/agentPools/%s",
			p.subscriptionID, p.resourceGroup, p.clusterName, apName)

		// Directly convert AgentPoolInfo to Instance
		instance, err = p.convertAgentPoolInfoToInstance(ctx, agentPoolInfo, instanceID)
		if err != nil {
			return err
		}

		logging.FromContext(ctx).Debugf("created agent pool %s", instanceID)
		return nil
	})
	if err != nil {
		return nil, err
	}

	instance, err := p.fromRegisteredAgentPoolToInstance(ctx, ap)
	if instance == nil && err == nil {
		// means the node object has not been found yet, we wait until the node is created
		b := wait.Backoff{
			Steps:    15,
			Duration: 1 * time.Second,
			Factor:   1.0,
			Jitter:   0.1,
		}

		err = retry.OnError(b, func(err error) bool {
			return true
		}, func() error {
			var e error
			instance, e = p.fromRegisteredAgentPoolToInstance(ctx, ap)
			if e != nil {
				return e
			}
			if instance == nil {
				return fmt.Errorf("fail to find the node object")
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		}
		return instance, err
	}

	return nil, fmt.Errorf("failed to create instance")
}

func (p *Provider) Get(ctx context.Context, id string) (*Instance, error) {
	apName, err := utils.ParseAgentPoolNameFromID(id)
	if err != nil {
		return nil, fmt.Errorf("getting agentpool name, %w", err)
	}

	agentPoolClient := p.extractAgentPoolClient()
	if agentPoolClient == nil {
		return nil, fmt.Errorf("unsupported client type")
	}

	params := AgentPoolParams{
		ResourceGroup: p.resourceGroup,
		ClusterName:   p.clusterName,
		AgentPoolName: apName,
	}

	agentPoolInfo, err := agentPoolClient.Get(ctx, params)
	if err != nil {
		if strings.Contains(err.Error(), "Agent Pool not found") {
			return nil, cloudprovider.NewNodeClaimNotFoundError(err)
		}
		logging.FromContext(ctx).Errorf("Get agentpool %q failed: %v", apName, err)
		return nil, fmt.Errorf("agentPool.Get for %s failed: %w", apName, err)
	}

	// Directly convert AgentPoolInfo to Instance
	return p.convertAgentPoolInfoToInstance(ctx, agentPoolInfo, id)
}

func (p *Provider) List(ctx context.Context) ([]*Instance, error) {
	agentPoolClient := p.extractAgentPoolClient()
	if agentPoolClient == nil {
		return nil, fmt.Errorf("unsupported client type")
	}

	params := AgentPoolParams{
		ResourceGroup: p.resourceGroup,
		ClusterName:   p.clusterName,
	}

	agentPoolInfos, err := agentPoolClient.List(ctx, params)
	if err != nil {
		logging.FromContext(ctx).Errorf("Listing agentpools failed: %v", err)
		return nil, fmt.Errorf("agentPool.NewListPager failed: %w", err)
	}

	// Directly convert AgentPoolInfos to Instances
	instances := []*Instance{}
	if len(agentPoolInfos) == 0 {
		return instances, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("agentpools not found"))
	}

	for _, info := range agentPoolInfos {
		// For filtering, we still need to check if it's owned by Kaito and created from NodeClaim
		// We can do this directly from AgentPoolInfo
		if !p.agentPoolInfoIsOwnedByKaito(info) {
			continue
		}

		if !p.agentPoolInfoIsCreatedFromNodeClaim(info) {
			continue
		}

		// Create a temporary ID for the instance (this mimics the old behavior)
		instanceID := fmt.Sprintf("azure:///subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s/agentPools/%s",
			p.subscriptionID, p.resourceGroup, p.clusterName, *info.Name)

		instance, err := p.convertAgentPoolInfoToInstanceWithNodes(ctx, info, instanceID)
		if err != nil {
			return instances, err
		}
		if instance != nil {
			instances = append(instances, instance)
		}
	}

	if len(instances) == 0 {
		return instances, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("agentpools not found"))
	}

	return instances, cloudprovider.IgnoreNodeClaimNotFoundError(nil)
}

func (p *Provider) Delete(ctx context.Context, apName string) error {
	klog.InfoS("Instance.Delete", "agentpool name", apName)

	agentPoolClient := p.extractAgentPoolClient()
	if agentPoolClient == nil {
		return fmt.Errorf("unsupported client type")
	}

	params := AgentPoolParams{
		ResourceGroup: p.resourceGroup,
		ClusterName:   p.clusterName,
		AgentPoolName: apName,
	}

	err := agentPoolClient.Delete(ctx, params)
	if err != nil {
		logging.FromContext(ctx).Errorf("Deleting agentpool %q failed: %v", apName, err)
		return fmt.Errorf("agentPool.Delete for %q failed: %w", apName, err)
	}
	return nil
}

// getNodesByName returns nodes with the specified agent pool name

func (p *Provider) fromRegisteredAgentPoolToInstance(ctx context.Context, apObj *armcontainerservice.AgentPool) (*Instance, error) {
	if apObj == nil {
		return nil, fmt.Errorf("agent pool is nil")
	}

	nodes, err := p.getNodesByName(ctx, lo.FromPtr(apObj.Name))
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 || len(nodes) > 1 {
		// NotFound is not considered as an error
		// and AgentPool may create more than one instance, we need to wait agentPool remove
		// the spare instance.
		return nil, nil
	}

	// we only want to resolve providerID and construct instance based on AgentPool.
	// there is no need to verify the node ready condition. so comment the following if condition.
	// if node == nil || nodeutil.GetCondition(node, v1.NodeReady).Status != v1.ConditionTrue {
	// 	// node is not found or not ready
	// 	return nil, nil
	// }

	// It's need to wait node and providerID ready when create AgentPool,
	// but there is no need to wait when termination controller lists all agentpools.
	// because termination controller garbage leaked agentpools.
	if len(nodes[0].Spec.ProviderID) == 0 {
		// provider id is not found
		return nil, nil
	}

	// tokens := strings.SplitAfter(node.Name, "-vmss") // remove the vm index "0000"
	instanceLabels := lo.MapValues(apObj.Properties.NodeLabels, func(k *string, _ string) string {
		return lo.FromPtr(k)
	})
	return &Instance{
		Name: apObj.Name,
		// ID:       to.Ptr(fmt.Sprint("azure://", p.getVMSSNodeProviderID(lo.FromPtr(subID), tokens[0]))),
		ID:       to.Ptr(nodes[0].Spec.ProviderID),
		Type:     apObj.Properties.VMSize,
		SubnetID: apObj.Properties.VnetSubnetID,
		Tags:     apObj.Properties.Tags,
		State:    apObj.Properties.ProvisioningState,
		Labels:   instanceLabels,
	}, nil
}

// fromKaitoAgentPoolToInstance is used to convert agentpool that owned by kaito to Instance, and agentPools that have no
// associated node are also included in order to garbage leaked agentPools.
func (p *Provider) fromKaitoAgentPoolToInstance(ctx context.Context, apObj *armcontainerservice.AgentPool) (*Instance, error) {
	if apObj == nil {
		return nil, fmt.Errorf("agent pool is nil")
	}

	instanceLabels := lo.MapValues(apObj.Properties.NodeLabels, func(k *string, _ string) string {
		return lo.FromPtr(k)
	})
	ins := &Instance{
		Name:     apObj.Name,
		Type:     apObj.Properties.VMSize,
		SubnetID: apObj.Properties.VnetSubnetID,
		Tags:     apObj.Properties.Tags,
		State:    apObj.Properties.ProvisioningState,
		Labels:   instanceLabels,
	}

	nodes, err := p.getNodesByName(ctx, lo.FromPtr(apObj.Name))
	if err != nil {
		return nil, err
	}

	if len(nodes) == 1 && len(nodes[0].Spec.ProviderID) != 0 {
		ins.ID = to.Ptr(nodes[0].Spec.ProviderID)
	}

	return ins, nil
}

func (p *Provider) fromAPListToInstances(ctx context.Context, apList []*armcontainerservice.AgentPool) ([]*Instance, error) {
	instances := []*Instance{}
	if len(apList) == 0 {
		return instances, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("agentpools not found"))
	}
	for index := range apList {
		// skip agentPool that is not owned by kaito
		if !agentPoolIsOwnedByKaito(apList[index]) {
			continue
		}

		// skip agentPool which is not created from nodeclaim
		if !agentPoolIsCreatedFromNodeClaim(apList[index]) {
			continue
		}

		instance, err := p.fromKaitoAgentPoolToInstance(ctx, apList[index])
		if err != nil {
			return instances, err
		}
		if instance != nil {
			instances = append(instances, instance)
		}
	}

	if len(instances) == 0 {
		return instances, cloudprovider.NewNodeClaimNotFoundError(fmt.Errorf("agentpools not found"))
	}

	return instances, nil
}

func newAgentPoolObject(vmSize string, nodeClaim *karpenterv1.NodeClaim) (armcontainerservice.AgentPool, error) {
	taints := nodeClaim.Spec.Taints
	taintsStr := []*string{}
	for _, t := range taints {
		taintsStr = append(taintsStr, to.Ptr(fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect)))
	}

	scaleSetsType := armcontainerservice.AgentPoolTypeVirtualMachineScaleSets
	// todo: why nodepool label is used here
	labels := map[string]*string{karpenterv1.NodePoolLabelKey: to.Ptr("kaito")}
	for k, v := range nodeClaim.Labels {
		labels[k] = to.Ptr(v)
	}

	if strings.Contains(vmSize, "Standard_N") {
		labels = lo.Assign(labels, map[string]*string{LabelMachineType: to.Ptr("gpu")})
	} else {
		labels = lo.Assign(labels, map[string]*string{LabelMachineType: to.Ptr("cpu")})
	}
	// NodeClaimCreationLabel is used for recording the create timestamp of agentPool resource.
	// then used by garbage collection controller to cleanup orphan agentpool which lived more than 10min
	labels[NodeClaimCreationLabel] = to.Ptr(nodeClaim.CreationTimestamp.UTC().Format(CreationTimestampLayout))

	storage := &resource.Quantity{}
	if nodeClaim.Spec.Resources.Requests != nil {
		storage = nodeClaim.Spec.Resources.Requests.Storage()
	}
	var diskSizeGB int32
	if storage.Value() <= 0 {
		return armcontainerservice.AgentPool{}, fmt.Errorf("storage request of nodeclaim(%s) should be more than 0", nodeClaim.Name)
	} else {
		diskSizeGB = int32(storage.Value() >> 30)
	}

	return armcontainerservice.AgentPool{
		Properties: &armcontainerservice.ManagedClusterAgentPoolProfileProperties{
			NodeLabels:   labels,
			NodeTaints:   taintsStr, //[]*string{to.Ptr("sku=gpu:NoSchedule")},
			Type:         to.Ptr(scaleSetsType),
			VMSize:       to.Ptr(vmSize),
			OSType:       to.Ptr(armcontainerservice.OSTypeLinux),
			Count:        to.Ptr(int32(1)),
			OSDiskSizeGB: to.Ptr(diskSizeGB),
		},
	}, nil
}

func (p *Provider) getNodesByName(ctx context.Context, apName string) ([]*v1.Node, error) {
	nodeList := &v1.NodeList{}
	labelSelector := client.MatchingLabels{"agentpool": apName, "kubernetes.azure.com/agentpool": apName}

	err := retry.OnError(retry.DefaultRetry, func(err error) bool {
		return true
	}, func() error {
		return p.kubeClient.List(ctx, nodeList, labelSelector)
	})
	if err != nil {
		return nil, err
	}

	return lo.ToSlicePtr(nodeList.Items), nil
}

func agentPoolIsOwnedByKaito(ap *armcontainerservice.AgentPool) bool {
	if ap == nil || ap.Properties == nil {
		return false
	}

	// when agentpool.NodeLabels includes labels from kaito, return true, if not, return false
	for i := range KaitoNodeLabels {
		if _, ok := ap.Properties.NodeLabels[KaitoNodeLabels[i]]; ok {
			return true
		}
	}

	return false
}

func agentPoolIsCreatedFromNodeClaim(ap *armcontainerservice.AgentPool) bool {
	if ap == nil || ap.Properties == nil {
		return false
	}

	// when agentpool.NodeLabels includes nodepool label, return true, if not, return false
	if _, ok := ap.Properties.NodeLabels[karpenterv1.NodePoolLabelKey]; ok {
		return true
	}

	return false
}

// extractAgentPoolClient extracts the AgentPoolClient from the azClient interface
func (p *Provider) extractAgentPoolClient() AgentPoolClient {
	switch c := p.azClient.(type) {
	case *AZClient:
		return c // AZClient now implements AgentPoolClient directly
	case *ArcAZClient:
		return c // ArcAZClient now implements AgentPoolClient directly
	case AgentPoolClient:
		return c // Already an AgentPoolClient
	default:
		return nil
	}
}

// buildAgentPoolParams creates AgentPoolParams from the provider's configuration
func (p *Provider) buildAgentPoolParams(agentPoolName string, agentPoolSpec armcontainerservice.AgentPool) AgentPoolParams {
	return AgentPoolParams{
		SubscriptionID: p.subscriptionID,
		ResourceGroup:  p.resourceGroup,
		ClusterName:    p.clusterName,
		AgentPoolName:  agentPoolName,
		AgentPoolSpec:  agentPoolSpec,
	}
}

// createInstanceFromAgentPoolObject creates a temporary instance from the original agent pool spec
func (p *Provider) createInstanceFromAgentPoolObject(apObj armcontainerservice.AgentPool, apName string) *Instance {
	instanceID := fmt.Sprintf("azure:///subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s/agentPools/%s",
		p.subscriptionID, p.resourceGroup, p.clusterName, apName)

	return &Instance{
		id:       instanceID,
		apName:   apName,
		apObj:    &apObj,
		vmSize:   *apObj.Properties.VMSize,
		location: p.location,
		status:   "Creating",
	}
}

// waitForNodeReady waits for the node to be ready and returns the final instance
func (p *Provider) waitForNodeReady(ctx context.Context, instance *Instance) (*Instance, error) {
	b := wait.Backoff{
		Steps:    15,
		Duration: 1 * time.Second,
		Factor:   1.0,
		Jitter:   0.1,
	}

	var finalInstance *Instance
	err := retry.OnError(b, func(err error) bool {
		return true
	}, func() error {
		// Try to get the updated instance from the API
		agentPoolClient := p.extractAgentPoolClient()
		if agentPoolClient == nil {
			return fmt.Errorf("unsupported client type")
		}

		agentPoolInfo, err := agentPoolClient.Get(ctx, instance.apName)
		if err != nil {
			return err
		}

		finalInstance, err = p.convertAgentPoolInfoToInstance(ctx, agentPoolInfo, instance.id)
		if err != nil {
			return err
		}

		// Check if we have node information
		if finalInstance.node == nil {
			return fmt.Errorf("node not ready yet")
		}

		return nil
	})

	return finalInstance, err
}

// convertAgentPoolInfoToInstance directly converts AgentPoolInfo to Instance
func (p *Provider) convertAgentPoolInfoToInstance(ctx context.Context, info *AgentPoolInfo, id string) (*Instance, error) {
	if info == nil {
		return nil, fmt.Errorf("agent pool info is nil")
	}

	// Convert map[string]*string to map[string]string for labels
	instanceLabels := make(map[string]string)
	for k, v := range info.NodeLabels {
		if v != nil {
			instanceLabels[k] = *v
		}
	}

	return &Instance{
		Name:     info.Name,
		ID:       to.Ptr(id),
		Type:     info.VMSize,
		SubnetID: info.VnetSubnetID,
		Tags:     info.Tags,
		State:    info.ProvisioningState,
		Labels:   instanceLabels,
		ImageID:  info.NodeImageVersion,
	}, nil
}

// agentPoolInfoIsOwnedByKaito checks if the agent pool was created from nodeclaim

// convertAgentPoolInfoToInstanceWithNodes converts AgentPoolInfo to Instance and handles node lookup
func (p *Provider) convertAgentPoolInfoToInstanceWithNodes(ctx context.Context, info *AgentPoolInfo, instanceID string) (*Instance, error) {
	if info == nil {
		return nil, fmt.Errorf("agent pool info is nil")
	}

	// Convert map[string]*string to map[string]string for labels
	instanceLabels := make(map[string]string)
	for k, v := range info.NodeLabels {
		if v != nil {
			instanceLabels[k] = *v
		}
	}

	ins := &Instance{
		Name:     info.Name,
		Type:     info.VMSize,
		SubnetID: info.VnetSubnetID,
		Tags:     info.Tags,
		State:    info.ProvisioningState,
		Labels:   instanceLabels,
		ImageID:  info.NodeImageVersion,
	}

	// Try to get the node to set the provider ID
	if info.Name != nil {
		nodes, err := p.getNodesByName(ctx, *info.Name)
		if err != nil {
			return nil, err
		}

		if len(nodes) == 1 && len(nodes[0].Spec.ProviderID) != 0 {
			ins.ID = to.Ptr(nodes[0].Spec.ProviderID)
		} else {
			ins.ID = to.Ptr(instanceID)
		}
	} else {
		ins.ID = to.Ptr(instanceID)
	}

	return ins, nil
}

// agentPoolInfoIsOwnedByKaito checks if AgentPoolInfo is owned by Kaito
func (p *Provider) agentPoolInfoIsOwnedByKaito(info *AgentPoolInfo) bool {
	if info == nil || info.NodeLabels == nil {
		return false
	}

	// when agentpool.NodeLabels includes labels from kaito, return true, if not, return false
	for i := range KaitoNodeLabels {
		if _, ok := info.NodeLabels[KaitoNodeLabels[i]]; ok {
			return true
		}
	}

	return false
}

// agentPoolInfoIsCreatedFromNodeClaim checks if AgentPoolInfo was created from NodeClaim
func (p *Provider) agentPoolInfoIsCreatedFromNodeClaim(info *AgentPoolInfo) bool {
	if info == nil || info.NodeLabels == nil {
		return false
	}

	// when agentpool.NodeLabels includes nodepool label, return true, if not, return false
	if _, ok := info.NodeLabels[karpenterv1.NodePoolLabelKey]; ok {
		return true
	}

	return false
}
