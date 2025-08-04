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
	"fmt"
	"os"
	"strings"

	"github.com/azure/gpu-provisioner/pkg/auth"
	"k8s.io/klog/v2"
)

// ClientFactory creates the appropriate Azure client based on environment configuration
type ClientFactory struct {
	clusterType ClusterType
	config      *auth.Config
}

// NewClientFactory creates a new factory instance
func NewClientFactory(config *auth.Config) *ClientFactory {
	// Get cluster type from environment variable
	clusterTypeStr := strings.ToLower(os.Getenv("AZURE_CLUSTER_TYPE"))
	if clusterTypeStr == "" {
		// Default to AKS for backward compatibility
		clusterTypeStr = string(ClusterTypeAKS)
		klog.V(2).InfoS("AZURE_CLUSTER_TYPE not set, defaulting to AKS")
	}

	clusterType := ClusterType(clusterTypeStr)
	klog.V(2).InfoS("Cluster type determined", "type", clusterType)

	return &ClientFactory{
		clusterType: clusterType,
		config:      config,
	}
}

// CreateAzClient creates the appropriate AZClient based on cluster type
func (f *ClientFactory) CreateAzClient() (interface{}, error) {
	switch f.clusterType {
	case ClusterTypeAKS:
		return f.createAKSClient()
	case ClusterTypeArc:
		return f.createArcClient()
	default:
		return nil, fmt.Errorf("unsupported cluster type: %s. Supported types are: %s, %s",
			f.clusterType, ClusterTypeAKS, ClusterTypeArc)
	}
}

// CreateAgentPoolClient creates the appropriate AgentPoolClient based on cluster type
func (f *ClientFactory) CreateAgentPoolClient() (AgentPoolClient, error) {
	switch f.clusterType {
	case ClusterTypeAKS:
		aksClient, err := f.createAKSClient()
		if err != nil {
			return nil, fmt.Errorf("failed to create AKS Azure client: %w", err)
		}
		// AZClient now implements AgentPoolClient directly
		return aksClient, nil
	case ClusterTypeArc:
		arcClient, err := f.createArcClient()
		if err != nil {
			return nil, fmt.Errorf("failed to create Arc Azure client: %w", err)
		}
		// ArcAZClient now implements AgentPoolClient directly
		return arcClient, nil
	default:
		return nil, fmt.Errorf("unsupported cluster type: %s", f.clusterType)
	}
}

// GetClusterType returns the current cluster type
func (f *ClientFactory) GetClusterType() ClusterType {
	return f.clusterType
}

func (f *ClientFactory) createAKSClient() (*AZClient, error) {
	klog.V(2).InfoS("Creating AKS client")
	return CreateAKSAzClient(f.config)
}

func (f *ClientFactory) createArcClient() (*ArcAZClient, error) {
	klog.V(2).InfoS("Creating Arc client")
	return CreateArcAzClient(f.config)
}
