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

package operator

import (
	"context"
	"fmt"
	"os"

	"github.com/azure/gpu-provisioner/pkg/auth"
	"github.com/azure/gpu-provisioner/pkg/providers"
	"github.com/azure/gpu-provisioner/pkg/providers/arcinstance"
	"github.com/azure/gpu-provisioner/pkg/providers/instance"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/karpenter/pkg/operator"
)

// Operator is injected into the CloudProvider's factories
type Operator struct {
	*operator.Operator
	InstanceProvider providers.InstanceProvider
}

func NewOperator(ctx context.Context, operator *operator.Operator) (context.Context, *Operator) {
	azConfig, err := GetAzConfig()
	if err != nil {
		logging.FromContext(ctx).Errorf("creating Azure config, %s", err)
	}

	// Get cloud provider type from environment variable
	cloudProvider := os.Getenv("CLOUD_PROVIDER")
	if cloudProvider == "" {
		cloudProvider = "aks" // default to AKS
	}

	var instanceProvider providers.InstanceProvider

	switch cloudProvider {
	case "aks":
		azClient, err := instance.CreateAzClient(azConfig)
		if err != nil {
			logging.FromContext(ctx).Errorf("creating Azure client, %s", err)
			// Let us panic here, instead of crashing in the following code.
			// TODO: move this to an init container
			panic(fmt.Sprintf("Configure azure client fails. Please ensure federatedcredential has been created for identity %s.", os.Getenv("AZURE_CLIENT_ID")))
		}

		instanceProvider = instance.NewProvider(
			azClient,
			operator.GetClient(),
			azConfig.ResourceGroup,
			azConfig.ClusterName,
		)

	case "arc":
		arcClient, err := arcinstance.NewArcClient(azConfig.SubscriptionID)
		if err != nil {
			logging.FromContext(ctx).Errorf("creating Arc client, %s", err)
			panic(fmt.Sprintf("Configure Arc client fails: %v", err))
		}

		instanceProvider = arcinstance.NewProvider(
			arcClient,
			operator.GetClient(),
			azConfig.SubscriptionID,
			azConfig.ResourceGroup,
			azConfig.ClusterName,
		)

	default:
		panic(fmt.Sprintf("Unsupported CLOUD_PROVIDER: %s. Supported values are 'aks' and 'arc'", cloudProvider))
	}

	logging.FromContext(ctx).Infof("Using cloud provider: %s", cloudProvider)

	return ctx, &Operator{
		Operator:         operator,
		InstanceProvider: instanceProvider,
	}
}

func GetAzConfig() (*auth.Config, error) {
	cfg, err := auth.BuildAzureConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
