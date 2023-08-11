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

package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/gpu-vmprovisioner/pkg/providers/instance"
)

type VirtualMachineCreateOrUpdateInput struct {
	ResourceGroupName string
	VMName            string
	VM                armcompute.VirtualMachine
	Options           *armcompute.VirtualMachinesClientBeginCreateOrUpdateOptions
}

type VirtualMachineDeleteInput struct {
	ResourceGroupName string
	VMName            string
	Options           *armcompute.VirtualMachinesClientBeginDeleteOptions
}

type VirtualMachineGetInput struct {
	ResourceGroupName string
	VMName            string
	Options           *armcompute.VirtualMachinesClientGetOptions
}

type VirtualMachinesBehavior struct {
	VirtualMachineCreateOrUpdateBehavior MockedLRO[VirtualMachineCreateOrUpdateInput, armcompute.VirtualMachinesClientCreateOrUpdateResponse]
	VirtualMachineDeleteBehavior         MockedLRO[VirtualMachineDeleteInput, armcompute.VirtualMachinesClientDeleteResponse]
	VirtualMachineGetBehavior            MockedFunction[VirtualMachineGetInput, armcompute.VirtualMachinesClientGetResponse]
	Instances                            sync.Map
}

// assert that the fake implements the interface
var _ instance.VirtualMachinesAPI = (*VirtualMachinesAPI)(nil)

type AgentPoolsAPI struct {
	// TODO
}

func (a AgentPoolsAPI) BeginCreateOrUpdate(ctx context.Context, resourceGroupName string, resourceName string, agentPoolName string, parameters armcontainerservice.AgentPool, options *armcontainerservice.AgentPoolsClientBeginCreateOrUpdateOptions) (*runtime.Poller[armcontainerservice.AgentPoolsClientCreateOrUpdateResponse], error) {
	//TODO implement me
	panic("implement me")
}

type VirtualMachinesAPI struct {
	// TODO: document the implications of embedding vs. not embedding the interface here
	// instance.VirtualMachinesAPI // - this is the interface we are mocking.
	VirtualMachinesBehavior
}

// Reset must be called between tests otherwise tests will pollute each other.
func (c *VirtualMachinesAPI) Reset() {
	c.VirtualMachineCreateOrUpdateBehavior.Reset()
	c.VirtualMachineDeleteBehavior.Reset()
	c.VirtualMachineGetBehavior.Reset()
	c.Instances.Range(func(k, v any) bool {
		c.Instances.Delete(k)
		return true
	})
}

func (c *VirtualMachinesAPI) BeginCreateOrUpdate(_ context.Context, resourceGroupName string, vmName string, parameters armcompute.VirtualMachine, options *armcompute.VirtualMachinesClientBeginCreateOrUpdateOptions) (*runtime.Poller[armcompute.VirtualMachinesClientCreateOrUpdateResponse], error) {
	// gather input parameters (may get rid of this with multiple mocked function signatures to reflect common patterns)
	input := &VirtualMachineCreateOrUpdateInput{
		ResourceGroupName: resourceGroupName,
		VMName:            vmName,
		VM:                parameters,
		Options:           options,
	}

	return c.VirtualMachineCreateOrUpdateBehavior.Invoke(input, func(input *VirtualMachineCreateOrUpdateInput) (*armcompute.VirtualMachinesClientCreateOrUpdateResponse, error) {
		// example of input validation
		//if input.ResourceGroupName == "" {
		//	return nil, errors.New("ResourceGroupName is required")
		//}
		// TODO: may have to clone ...
		// TODO: subscription ID?
		vm := input.VM
		id := mkVMID(input.ResourceGroupName, input.VMName)
		vm.ID = to.StringPtr(id)
		vm.Name = to.StringPtr(input.VMName)
		timeCreated := time.Now() // TODO: use simulated time?
		if vm.Properties == nil {
			vm.Properties = &armcompute.VirtualMachineProperties{}
		}
		vm.Properties.TimeCreated = &timeCreated
		c.Instances.Store(id, vm)
		return &armcompute.VirtualMachinesClientCreateOrUpdateResponse{
			VirtualMachine: vm,
		}, nil
	})
}

func (c *VirtualMachinesAPI) Get(_ context.Context, resourceGroupName string, vmName string, options *armcompute.VirtualMachinesClientGetOptions) (armcompute.VirtualMachinesClientGetResponse, error) {
	input := &VirtualMachineGetInput{
		ResourceGroupName: resourceGroupName,
		VMName:            vmName,
		Options:           options,
	}
	return c.VirtualMachineGetBehavior.Invoke(input, func(input *VirtualMachineGetInput) (armcompute.VirtualMachinesClientGetResponse, error) {
		instance, _ := c.Instances.Load(mkVMID(input.ResourceGroupName, input.VMName))
		return armcompute.VirtualMachinesClientGetResponse{
			VirtualMachine: instance.(armcompute.VirtualMachine),
		}, nil
	})
}

func (c *VirtualMachinesAPI) BeginDelete(_ context.Context, resourceGroupName string, vmName string, options *armcompute.VirtualMachinesClientBeginDeleteOptions) (*runtime.Poller[armcompute.VirtualMachinesClientDeleteResponse], error) {
	input := &VirtualMachineDeleteInput{
		ResourceGroupName: resourceGroupName,
		VMName:            vmName,
		Options:           options,
	}
	return c.VirtualMachineDeleteBehavior.Invoke(input, func(input *VirtualMachineDeleteInput) (*armcompute.VirtualMachinesClientDeleteResponse, error) {
		c.Instances.Delete(mkVMID(input.ResourceGroupName, input.VMName))
		return &armcompute.VirtualMachinesClientDeleteResponse{}, nil
	})
}

func mkVMID(resourceGroupName string, vmName string) string {
	const idFormat = "/subscriptions/subscriptionID/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s"
	return fmt.Sprintf(idFormat, resourceGroupName, vmName)
}
