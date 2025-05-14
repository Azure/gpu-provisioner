package instance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/azure/gpu-provisioner/pkg/auth"
	"github.com/azure/gpu-provisioner/pkg/fake"
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
