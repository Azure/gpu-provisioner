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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Azure contains parameters specific to this cloud provider
// +kubebuilder:object:root=true
type Azure struct {
	// TypeMeta includes version and kind of the extensions, inferred if not provided.
	// +optional
	metav1.TypeMeta `json:",inline"`
	// ImageID is the ImageVersion that the instances use.
	// +optional
	ImageID *string `json:"imageID,omitempty"`
	// ImageFamily is the image family that instances use.
	// Tags to be applied on Azure resources like instances.
	// +optional
	Tags map[string]string `json:"tags,omitempty"`
}

func DeserializeProvider(raw []byte) (*Azure, error) {
	a := &Azure{}
	_, gvk, err := codec.UniversalDeserializer().Decode(raw, nil, a)
	if err != nil {
		return nil, err
	}
	if gvk != nil {
		a.SetGroupVersionKind(*gvk)
	}
	return a, nil
}
