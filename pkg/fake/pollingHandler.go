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
// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime (interfaces: PagingHandler)

package fake

import (
	"context"
	"net/http"
	"reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockPollingHandler is a mock implementation of PollingHandler interface.
type MockPollingHandler[T any] struct {
	ctrl     *gomock.Controller
	recorder *MockPollingHandlerMockRecorder[T]
}

// MockPollingHandlerMockRecorder is the mock recorder for MockPollingHandler.
type MockPollingHandlerMockRecorder[T any] struct {
	mock *MockPollingHandler[T]
}

// NewMockPollingHandler creates a new mock instance.
func NewMockPollingHandler[T any](ctrl *gomock.Controller) *MockPollingHandler[T] {
	mock := &MockPollingHandler[T]{ctrl: ctrl}
	mock.recorder = &MockPollingHandlerMockRecorder[T]{mock}
	return mock
}

// EXPECT methods can be used to set up expected calls.
func (m *MockPollingHandler[T]) EXPECT() *MockPollingHandlerMockRecorder[T] {
	return m.recorder
}

// Done mocks the Done method of the PollingHandler interface.
func (m *MockPollingHandler[T]) Done() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Done")
	return ret[0].(bool)
}

func (mr *MockPollingHandlerMockRecorder[T]) Done() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Done", reflect.TypeOf((*MockPollingHandler[T])(nil).Done))
}

// Poll mocks the Poll method of the PollingHandler interface.
func (m *MockPollingHandler[T]) Poll(ctx context.Context) (*http.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Poll", ctx)
	return ret[0].(*http.Response), ret[1].(error)
}

func (mr *MockPollingHandlerMockRecorder[T]) Poll(ctx any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Poll", reflect.TypeOf((*MockPollingHandler[T])(nil).Poll), ctx)
}

// Result mocks the Result method of the PollingHandler interface.
func (m *MockPollingHandler[T]) Result(ctx context.Context, out *T) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Result", ctx, out)
	err, ok := ret[0].(error)

	if !ok {
		return nil
	}
	return err
}

func (mr *MockPollingHandlerMockRecorder[T]) Result(ctx, out any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Result", reflect.TypeOf((*MockPollingHandler[T])(nil).Result), ctx, out)
}
