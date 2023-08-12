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

package cloudprovider

import (
	"context"
	"fmt"
	"net/http"

	v1 "k8s.io/api/core/v1"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gpu-vmprovisioner/pkg/apis"
	"github.com/gpu-vmprovisioner/pkg/providers/instance"
	"github.com/gpu-vmprovisioner/pkg/providers/instancetype"
	"github.com/gpu-vmprovisioner/pkg/utils"
	"github.com/samber/lo"

	coreapis "github.com/aws/karpenter-core/pkg/apis"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
	"github.com/aws/karpenter-core/pkg/scheduling"
	"github.com/gpu-vmprovisioner/pkg/staticprovisioner"
)

func init() {
	// TODO: add Azure aliased labels, if any
	// v1alpha5.NormalizedLabels = lo.Assign(v1alpha5.NormalizedLabels, ...)

	// change the non-spot priority/capacity name known to Karpenter core
	// from "on-demand" to "regular" (currently only affects consolidation)
	// TODO: was changed to constant, so can't override now ...
	// v1alpha5.CapacityTypeOnDemand = v1alpha1.PriorityRegular

	coreapis.Settings = append(coreapis.Settings, apis.Settings...)
}

var _ cloudprovider.CloudProvider = (*CloudProvider)(nil)

type CloudProvider struct {
	instanceTypeProvider *instancetype.Provider
	instanceProvider     *instance.Provider
	kubeClient           client.Client
}

func New(instanceTypeProvider *instancetype.Provider, instanceProvider *instance.Provider, kubeClient client.Client) *CloudProvider {
	return &CloudProvider{
		instanceTypeProvider: instanceTypeProvider,
		instanceProvider:     instanceProvider,
		kubeClient:           kubeClient,
	}
}

// Create a node given the constraints.
func (c *CloudProvider) Create(ctx context.Context, machine *v1alpha5.Machine) (*v1alpha5.Machine, error) {
	instanceTypes, err := c.resolveInstanceTypes(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("resolving instance types, %w", err)
	}
	if len(instanceTypes) == 0 {
		return nil, fmt.Errorf("all requested instance types were unavailable during launch")
	}
	instance, err := c.instanceProvider.Create(ctx, machine, instanceTypes)
	if err != nil {
		return nil, fmt.Errorf("creating instance, %w", err)
	}
	_, _ = lo.Find(instanceTypes, func(i *cloudprovider.InstanceType) bool {
		return i.Name == *instance.Type // vm size
	})

	return nil, nil
}

func (c *CloudProvider) List(ctx context.Context) ([]*v1alpha5.Machine, error) {
	_, err := c.instanceProvider.List(ctx)
	return nil, err
}

func (c *CloudProvider) Get(ctx context.Context, providerID string) (*v1alpha5.Machine, error) {
	id, err := utils.ParseInstanceID(providerID)
	if err != nil {
		return nil, fmt.Errorf("getting instance ID, %w", err)
	}
	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With("id", id))
	_, err = c.instanceProvider.Get(ctx, id)

	return nil, err
}

func (c *CloudProvider) LivenessProbe(req *http.Request) error {
	return c.instanceTypeProvider.LivenessProbe(req)
}

// GetInstanceTypes returns all available InstanceTypes
func (c *CloudProvider) GetInstanceTypes(ctx context.Context, provisioner *v1alpha5.Provisioner) ([]*cloudprovider.InstanceType, error) {
	provisioner = staticprovisioner.Sp
	if provisioner.Spec.ProviderRef == nil {
		return nil, nil
	}

	instanceTypes, err := c.instanceTypeProvider.List(ctx, provisioner.Spec.KubeletConfiguration)
	if err != nil {
		return nil, err
	}
	return instanceTypes, nil
}

func (c *CloudProvider) Delete(ctx context.Context, machine *v1alpha5.Machine) error {
	return c.instanceProvider.Delete(ctx, machine.Name)
}

func (c *CloudProvider) IsMachineDrifted(ctx context.Context, machine *v1alpha5.Machine) (bool, error) {
	// Not needed when GetInstanceTypes removes provisioner dependency
	//provisioner := &v1alpha5.Provisioner{}
	//if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: machine.Labels[v1alpha5.ProvisionerNameLabelKey]}, provisioner); err != nil {
	//	return false, client.IgnoreNotFound(fmt.Errorf("getting provisioner, %w", err))
	//}
	provisioner := staticprovisioner.Sp
	if provisioner.Spec.ProviderRef == nil {
		return false, nil
	}

	imageDrifted, err := c.isImageDrifted(ctx, machine, provisioner)
	if err != nil {
		return false, err
	}
	return imageDrifted, nil
}

// Name returns the CloudProvider implementation name.
func (c *CloudProvider) Name() string {
	return "azure"
}

// TODO: remove nolint on unparam. Added for now in order to pass "make verify" in azure/poc
// nolint: unparam
func (c *CloudProvider) isImageDrifted(
	ctx context.Context, machine *v1alpha5.Machine, provisioner *v1alpha5.Provisioner) (bool, error) {
	instanceTypes, err := c.GetInstanceTypes(ctx, provisioner)
	if err != nil {
		return false, fmt.Errorf("getting instanceTypes, %w", err)
	}
	_, found := lo.Find(instanceTypes, func(instType *cloudprovider.InstanceType) bool {
		return instType.Name == machine.Labels[v1.LabelInstanceTypeStable]
	})
	if !found {
		return false, fmt.Errorf(`finding node instance type "%s"`, machine.Labels[v1.LabelInstanceTypeStable])
	}

	return false, nil
}

func (c *CloudProvider) resolveInstanceTypes(ctx context.Context, machine *v1alpha5.Machine) ([]*cloudprovider.InstanceType, error) {
	//provisionerName, ok := machine.Labels[v1alpha5.ProvisionerNameLabelKey]
	//if !ok {
	//	return nil, fmt.Errorf("finding provisioner owner")
	//}
	//provisioner := &v1alpha5.Provisioner{}
	//if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: provisionerName}, provisioner); err != nil {
	//	return nil, fmt.Errorf("getting provisioner owner, %w", err)
	//}
	provisioner := staticprovisioner.Sp
	instanceTypes, err := c.GetInstanceTypes(ctx, provisioner)
	if err != nil {
		return nil, fmt.Errorf("getting instance types, %w", err)
	}
	reqs := scheduling.NewNodeSelectorRequirements(machine.Spec.Requirements...)
	return lo.Filter(instanceTypes, func(i *cloudprovider.InstanceType, _ int) bool {
		return reqs.Get(v1.LabelInstanceTypeStable).Has(i.Name) && len(i.Offerings.Requirements(reqs).Available()) > 0
	}), nil
}

func (c *CloudProvider) resolveInstanceTypeFromInstance(ctx context.Context, instance *v1alpha5.Machine) (*cloudprovider.InstanceType, error) {
	provisioner, err := c.resolveProvisionerFromInstance(ctx, instance)
	if err != nil {
		// If we can't resolve the provisioner, we fallback to not getting instance type info
		return nil, client.IgnoreNotFound(fmt.Errorf("resolving provisioner, %w", err))
	}
	instanceTypes, err := c.GetInstanceTypes(ctx, provisioner)
	if err != nil {
		// If we can't resolve the provisioner, we fallback to not getting instance type info
		return nil, client.IgnoreNotFound(fmt.Errorf("resolving node template, %w", err))
	}
	instanceType, _ := lo.Find(instanceTypes, func(i *cloudprovider.InstanceType) bool {
		return i.Name == instance.Spec.MachineTemplateRef.Name
	})
	return instanceType, nil
}

func (c *CloudProvider) resolveProvisionerFromInstance(ctx context.Context, instance *v1alpha5.Machine) (*v1alpha5.Provisioner, error) {
	//	provisioner := &v1alpha5.Provisioner{}
	//	provisionerName, ok := instance.Tags[v1alpha5.ProvisionerNameLabelKey]
	//	if !ok {
	//		return nil, errors.NewNotFound(schema.GroupResource{Group: v1alpha5.Group, Resource: "Provisioner"}, "")
	//	}
	//	if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: *provisionerName}, provisioner); err != nil {
	//		return nil, err
	//	}
	provisioner := staticprovisioner.Sp
	return provisioner, nil
}
