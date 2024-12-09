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

package state

import (
	"github.com/prometheus/client_golang/prometheus"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"sigs.k8s.io/karpenter/pkg/metrics"
)

const (
	stateSubsystem = "cluster_state"
)

var (
	ClusterStateNodesCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metrics.Namespace,
			Subsystem: stateSubsystem,
			Name:      "node_count",
			Help:      "Current count of nodes in cluster state",
		},
	)

	ClusterStateSynced = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metrics.Namespace,
			Subsystem: stateSubsystem,
			Name:      "synced",
			Help:      "Returns 1 if cluster state is synced and 0 otherwise. Synced checks that nodeclaims and nodes that are stored in the APIServer have the same representation as Karpenter's cluster state",
		},
	)
)

func init() {
	crmetrics.Registry.MustRegister(ClusterStateNodesCount, ClusterStateSynced)
}
