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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KaitoNodeClass is the Schema for the KaitoNodeClass API
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kaitonodeclasses,scope=Cluster,categories=karpenter,shortName={knc}
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type KaitoNodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KaitoNodeClassSpec   `json:"spec,omitempty"`
	Status KaitoNodeClassStatus `json:"status,omitempty"`
}

type KaitoNodeClassSpec struct {
	// Add fields here
}

type KaitoNodeClassStatus struct {
	// Add fields here
}

// KaitoNodeClassList contains a list of KaitoNodeClass
// +kubebuilder:object:root=true
type KaitoNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KaitoNodeClass `json:"items"`
}
