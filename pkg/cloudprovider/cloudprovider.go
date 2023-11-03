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

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/azure/gpu-provisioner/pkg/apis"
	"github.com/azure/gpu-provisioner/pkg/providers/instance"
	"github.com/azure/gpu-provisioner/pkg/providers/instancetype"
	"github.com/samber/lo"

	coreapis "github.com/aws/karpenter-core/pkg/apis"
	"github.com/aws/karpenter-core/pkg/apis/v1alpha5"
	"github.com/aws/karpenter-core/pkg/cloudprovider"
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
	klog.InfoS("Create", "machine", klog.KObj(machine))

	instance, err := c.instanceProvider.Create(ctx, machine)
	if err != nil {
		return nil, fmt.Errorf("creating instance, %w", err)
	}
	m := c.instanceToMachine(ctx, instance)
	m.Labels = lo.Assign(m.Labels, instance.Labels)
	return m, nil
}

func (c *CloudProvider) List(ctx context.Context) ([]*v1alpha5.Machine, error) {
	machines := []*v1alpha5.Machine{}
	instances, err := c.instanceProvider.List(ctx)
	if err != nil {
		return nil, err
	}

	for index := range instances {
		machines = append(machines, c.instanceToMachine(ctx, instances[index]))
	}
	return machines, nil
}

func (c *CloudProvider) Get(ctx context.Context, providerID string) (*v1alpha5.Machine, error) {
	klog.InfoS("Get", "providerID", providerID)

	instance, err := c.instanceProvider.Get(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("getting instance , %w", err)
	}
	if instance == nil {
		return nil, fmt.Errorf("cannot find a ready instance , %w", err)
	}
	return c.instanceToMachine(ctx, instance), err
}

func (c *CloudProvider) LivenessProbe(req *http.Request) error {
	return c.instanceTypeProvider.LivenessProbe(req)
}

func (c *CloudProvider) Delete(ctx context.Context, machine *v1alpha5.Machine) error {
	klog.InfoS("Delete", "machine", klog.KObj(machine))
	return c.instanceProvider.Delete(ctx, machine.Status.ProviderID)
}

func (c *CloudProvider) IsMachineDrifted(ctx context.Context, machine *v1alpha5.Machine) (bool, error) {
	klog.InfoS("IsMachineDrifted", "machine", klog.KObj(machine))
	return false, nil
}

func (c *CloudProvider) GetInstanceTypes(ctx context.Context, provisioner *v1alpha5.Provisioner) ([]*cloudprovider.InstanceType, error) {

	instanceTypes := []*cloudprovider.InstanceType{}

	return instanceTypes, nil
}

// Name returns the CloudProvider implementation name.
func (c *CloudProvider) Name() string {
	return "azure"
}
