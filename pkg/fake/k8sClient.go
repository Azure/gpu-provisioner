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

package fake

import (
	"context"
	"fmt"
	"reflect"

	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// MockClient is a mock for the controller-runtime dynamic client interface.
type MockClient struct {
	mock.Mock

	ObjectMap  map[reflect.Type]map[k8sClient.ObjectKey]k8sClient.Object
	StatusMock *MockStatusClient
	UpdateCb   func(key types.NamespacedName)
}

var _ k8sClient.Client = &MockClient{}

func NewClient() *MockClient {
	return &MockClient{
		StatusMock: &MockStatusClient{},
		ObjectMap:  map[reflect.Type]map[k8sClient.ObjectKey]k8sClient.Object{},
	}
}

// Retrieves or creates a map associated with the type of obj
func (m *MockClient) ensureMapForType(t reflect.Type) map[k8sClient.ObjectKey]k8sClient.Object {
	if _, ok := m.ObjectMap[t]; !ok {
		//create a new map with the object key if it doesn't exist
		m.ObjectMap[t] = map[k8sClient.ObjectKey]k8sClient.Object{}
	}
	return m.ObjectMap[t]
}

func (m *MockClient) CreateMapWithType(t interface{}) map[k8sClient.ObjectKey]k8sClient.Object {
	objType := reflect.TypeOf(t)

	return m.ensureMapForType(objType)
}

func (m *MockClient) CreateOrUpdateObjectInMap(obj k8sClient.Object) {
	t := reflect.TypeOf(obj)
	relevantMap := m.ensureMapForType(t)
	objKey := k8sClient.ObjectKeyFromObject(obj)

	relevantMap[objKey] = obj
}

func (m *MockClient) GetObjectFromMap(obj k8sClient.Object, key types.NamespacedName) {
	t := reflect.TypeOf(obj)
	relevantMap := m.ensureMapForType(t)

	if val, ok := relevantMap[key]; ok {
		v := reflect.ValueOf(obj).Elem()
		v.Set(reflect.ValueOf(val).Elem())
	}
}

// k8s Client interface
func (m *MockClient) Get(ctx context.Context, key types.NamespacedName, obj k8sClient.Object, opts ...k8sClient.GetOption) error {
	//make any necessary changes to the object
	if m.UpdateCb != nil {
		m.UpdateCb(key)
	}

	m.GetObjectFromMap(obj, key)

	args := m.Called(ctx, key, obj, opts)
	return args.Error(0)
}

func (m *MockClient) List(ctx context.Context, list k8sClient.ObjectList, opts ...k8sClient.ListOption) error {

	v := reflect.ValueOf(list).Elem()
	newList := m.getObjectListFromMap(list)
	v.Set(reflect.ValueOf(newList).Elem())

	args := m.Called(ctx, list, opts)
	return args.Error(0)
}

func (m *MockClient) getObjectListFromMap(list k8sClient.ObjectList) k8sClient.ObjectList {
	objType := reflect.TypeOf(list)
	relevantMap := m.ensureMapForType(objType)

	switch list.(type) {
	case *corev1.NodeList:
		nodeList := &corev1.NodeList{}
		for _, obj := range relevantMap {
			if node, ok := obj.(*corev1.Node); ok {
				nodeList.Items = append(nodeList.Items, *node)
			}
		}
		return nodeList
	}
	//add additional object lists as needed
	return nil
}

func (m *MockClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...k8sClient.ApplyOption) error {
	return nil
}

func (m *MockClient) Create(ctx context.Context, obj k8sClient.Object, opts ...k8sClient.CreateOption) error {
	m.CreateOrUpdateObjectInMap(obj)

	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Delete(ctx context.Context, obj k8sClient.Object, opts ...k8sClient.DeleteOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Update(ctx context.Context, obj k8sClient.Object, opts ...k8sClient.UpdateOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func (m *MockClient) Patch(ctx context.Context, obj k8sClient.Object, patch k8sClient.Patch, opts ...k8sClient.PatchOption) error {
	args := m.Called(ctx, obj, patch, opts)
	return args.Error(0)
}

func (m *MockClient) DeleteAllOf(ctx context.Context, obj k8sClient.Object, opts ...k8sClient.DeleteAllOfOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

// GroupVersionKindFor implements client.Client
func (m *MockClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	panic("unimplemented")
}

// IsObjectNamespaced implements client.Client
func (m *MockClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	panic("unimplemented")
}

func (m *MockClient) Scheme() *runtime.Scheme {
	args := m.Called()
	return args.Get(0).(*runtime.Scheme)
}

func (m *MockClient) RESTMapper() meta.RESTMapper {
	args := m.Called()
	return args.Get(0).(meta.RESTMapper)
}

// StatusClient interface
func (m *MockClient) Status() k8sClient.StatusWriter {
	return m.StatusMock
}

// SubResourceClientConstructor interface
func (m *MockClient) SubResource(subResource string) k8sClient.SubResourceClient {
	return m.StatusMock
}

type MockStatusClient struct {
	mock.Mock
}

// Create implements client.StatusWriter
func (*MockStatusClient) Create(ctx context.Context, obj k8sClient.Object, subResource k8sClient.Object, opts ...k8sClient.SubResourceCreateOption) error {
	panic("unimplemented")
}

// Patch implements client.StatusWriter
func (*MockStatusClient) Patch(ctx context.Context, obj k8sClient.Object, patch k8sClient.Patch, opts ...k8sClient.SubResourcePatchOption) error {
	panic("unimplemented")
}

// Update implements client.StatusWriter
func (*MockStatusClient) Update(ctx context.Context, obj k8sClient.Object, opts ...k8sClient.SubResourceUpdateOption) error {
	panic("unimplemented")
}

func (*MockStatusClient) Get(ctx context.Context, obj k8sClient.Object, subResource k8sClient.Object, opts ...k8sClient.SubResourceGetOption) error {
	panic("unimplemented")
}

func CreateNodeListWithNodeClaim(nodeClaims []*karpenterv1.NodeClaim) *corev1.NodeList {
	nodes := []corev1.Node{}
	for i := range nodeClaims {
		if len(nodeClaims[i].Status.ProviderID) == 0 {
			continue
		}
		nodes = append(nodes, corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("aks-%s-20562481-vmss_0", nodeClaims[i].Name),
				Labels: map[string]string{
					"agentpool":                      nodeClaims[i].Name,
					"kubernetes.azure.com/agentpool": nodeClaims[i].Name,
				},
			},
			Spec: corev1.NodeSpec{
				ProviderID: nodeClaims[i].Status.ProviderID,
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		})
	}

	return &corev1.NodeList{
		Items: nodes,
	}
}
