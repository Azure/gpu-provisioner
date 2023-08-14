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
	instanceType, _ := lo.Find(instanceTypes, func(i *cloudprovider.InstanceType) bool {
		return i.Name == *instance.Type // vm size
	})

	m := c.instanceToMachine(ctx, instance, instanceType)

	return m, nil
}

func (c *CloudProvider) List(ctx context.Context) ([]*v1alpha5.Machine, error) {
	var machines []*v1alpha5.Machine
	instances, err := c.instanceProvider.List(ctx)
	instanceTypes, err := c.GetInstanceTypes(ctx, staticprovisioner.Sp)
	if err != nil {
		return nil, fmt.Errorf("getting instance types, %w", err)
	}

	for index := range instances {
		instanceType, _ := lo.Find(instanceTypes, func(instanceType *cloudprovider.InstanceType) bool {
			return instanceType.Name == *instances[index].Type // vm size
		})
		machines = append(machines, c.instanceToMachine(ctx, instances[index], instanceType))
	}
	return machines, err
}

func (c *CloudProvider) Get(ctx context.Context, providerID string) (*v1alpha5.Machine, error) {
	id, err := utils.ParseInstanceID(providerID)
	if err != nil {
		return nil, fmt.Errorf("getting instance ID, %w", err)
	}
	ctx = logging.WithLogger(ctx, logging.FromContext(ctx).With("id", id))
	instance, err := c.instanceProvider.Get(ctx, *id)

	instanceTypes, err := c.GetInstanceTypes(ctx, staticprovisioner.Sp)
	if err != nil {
		return nil, fmt.Errorf("getting instance types, %w", err)
	}
	instanceType, _ := lo.Find(instanceTypes, func(instanceType *cloudprovider.InstanceType) bool {
		return instanceType.Name == *instance.Type // vm size
	})
	return c.instanceToMachine(ctx, instance, instanceType), err
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
	instanceTypes, err := c.GetInstanceTypes(ctx, staticprovisioner.Sp)
	if err != nil {
		return nil, fmt.Errorf("getting instance types, %w", err)
	}
	reqs := scheduling.NewNodeSelectorRequirements(machine.Spec.Requirements...)
	return lo.Filter(instanceTypes, func(i *cloudprovider.InstanceType, _ int) bool {
		return reqs.Get(v1.LabelInstanceTypeStable).Has(i.Name) && len(i.Offerings.Requirements(reqs).Available()) > 0
	}), nil
}
