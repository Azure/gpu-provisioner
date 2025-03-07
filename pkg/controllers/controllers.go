/*
Copyright The Kubernetes Authors.

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

package controllers

import (
	"github.com/awslabs/operatorpkg/controller"
	instancegarbagecollection "github.com/azure/gpu-provisioner/pkg/controllers/instance/garbagecollection"
	nodeclaimstatus "github.com/azure/gpu-provisioner/pkg/controllers/nodeclaim"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

func NewControllers(kubeClient client.Client, cloudProvider cloudprovider.CloudProvider) []controller.Controller {
	controllers := []controller.Controller{
		instancegarbagecollection.NewController(kubeClient, cloudProvider),
		nodeclaimstatus.NewController(kubeClient),
	}
	return controllers
}
