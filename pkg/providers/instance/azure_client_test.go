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
	"testing"

	"github.com/azure/gpu-provisioner/pkg/auth"
	"github.com/azure/gpu-provisioner/pkg/fake"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// Test NewAZClientFromAPI with the generated mock
func TestNewAZClientFromAPI(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAPI := fake.NewMockAgentPoolsAPI(ctrl)
	client := NewAZClientFromAPI(mockAPI)
	assert.NotNil(t, client)
	assert.Equal(t, mockAPI, client.agentPoolsClient)
}

// Test CreateAzClient with invalid deployment mode
func TestCreateAzClient_InvalidDeploymentMode(t *testing.T) {
	cfg := &auth.Config{DeploymentMode: "invalid"}
	_, err := CreateAzClient(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid deployment mode")
}
