package vault

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/panteparak/vault-autounseal-operator/test/unit/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ClientMockTestSuite provides unit testing for vault client functionality using mocks
type ClientMockTestSuite struct {
	suite.Suite
	ctx        context.Context
	mockClient *mocks.MockVaultClient
}

func (suite *ClientMockTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.mockClient = new(mocks.MockVaultClient)
}

func (suite *ClientMockTestSuite) TearDownTest() {
	if suite.mockClient != nil {
		suite.mockClient.AssertExpectations(suite.T())
	}
}

// TestMockClientBasicOperations tests basic client operations with mocks
func (suite *ClientMockTestSuite) TestMockClientBasicOperations() {
	// Setup healthy vault mock
	mocks.SetupHealthyVault(suite.mockClient)

	// Test basic properties (only interface methods)
	assert.False(suite.T(), suite.mockClient.IsClosed())

	// Test health check
	health, err := suite.mockClient.HealthCheck(suite.ctx)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), health)
	assert.False(suite.T(), health.Sealed)

	// Test seal status
	isSealed, err := suite.mockClient.IsSealed(suite.ctx)
	require.NoError(suite.T(), err)
	assert.False(suite.T(), isSealed)

	// Test initialization status
	isInit, err := suite.mockClient.IsInitialized(suite.ctx)
	require.NoError(suite.T(), err)
	assert.True(suite.T(), isInit)

	// Test close
	err = suite.mockClient.Close()
	require.NoError(suite.T(), err)
}

// TestMockClientSealedVault tests operations on a sealed vault
func (suite *ClientMockTestSuite) TestMockClientSealedVault() {
	// Setup sealed vault mock
	mocks.SetupSealedVault(suite.mockClient)

	// Test seal status
	isSealed, err := suite.mockClient.IsSealed(suite.ctx)
	require.NoError(suite.T(), err)
	assert.True(suite.T(), isSealed)

	// Test seal status response
	sealStatus, err := suite.mockClient.GetSealStatus(suite.ctx)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), sealStatus)
	assert.True(suite.T(), sealStatus.Sealed)
	assert.Equal(suite.T(), 3, sealStatus.T) // threshold
	assert.Equal(suite.T(), 0, sealStatus.Progress)
}

// TestMockClientFailingVault tests error handling with a failing vault
func (suite *ClientMockTestSuite) TestMockClientFailingVault() {
	// Setup failing vault mock
	mocks.SetupFailingVault(suite.mockClient, "connection refused")

	// All operations should return errors
	_, err := suite.mockClient.IsSealed(suite.ctx)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "connection refused")

	_, err = suite.mockClient.IsInitialized(suite.ctx)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "connection refused")

	_, err = suite.mockClient.HealthCheck(suite.ctx)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "connection refused")

	_, err = suite.mockClient.GetSealStatus(suite.ctx)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "connection refused")
}

// TestMockClientUnsealingSequence tests complete unsealing sequence
func (suite *ClientMockTestSuite) TestMockClientUnsealingSequence() {
	threshold := 3
	keys := []string{
		"ZGVmYXVsdC11bnNlYWwta2V5LTEtZm9yLXRlc3Rpbmc=",
		"ZGVmYXVsdC11bnNlYWwta2V5LTItZm9yLXRlc3Rpbmc=",
		"ZGVmYXVsdC11bnNlYWwta2V5LTMtZm9yLXRlc3Rpbmc=",
	}

	// Setup unsealing sequence mock
	mocks.SetupUnsealingSequence(suite.mockClient, threshold)

	// Initial state should be sealed
	isSealed, err := suite.mockClient.IsSealed(suite.ctx)
	require.NoError(suite.T(), err)
	assert.True(suite.T(), isSealed)

	// Submit keys one by one
	for i, key := range keys {
		response, err := suite.mockClient.SubmitSingleKey(suite.ctx, key, i)
		require.NoError(suite.T(), err)
		require.NotNil(suite.T(), response)

		if i < threshold-1 {
			// Still sealed until we reach threshold
			assert.True(suite.T(), response.Sealed)
			assert.Equal(suite.T(), i+1, response.Progress)
		} else {
			// Should be unsealed after reaching threshold
			assert.False(suite.T(), response.Sealed)
			assert.Equal(suite.T(), threshold, response.Progress)
		}
	}

	// Final state should be unsealed
	isSealed, err = suite.mockClient.IsSealed(suite.ctx)
	require.NoError(suite.T(), err)
	assert.False(suite.T(), isSealed)
}

// TestMockClientUnsealWithMultipleKeys tests unsealing with multiple keys at once
func (suite *ClientMockTestSuite) TestMockClientUnsealWithMultipleKeys() {
	threshold := 3
	keys := []string{
		"ZGVmYXVsdC11bnNlYWwta2V5LTEtZm9yLXRlc3Rpbmc=",
		"ZGVmYXVsdC11bnNlYWwta2V5LTItZm9yLXRlc3Rpbmc=",
		"ZGVmYXVsdC11bnNlYWwta2V5LTMtZm9yLXRlc3Rpbmc=",
	}

	// Setup mock for unsealing with multiple keys
	suite.mockClient.On("Unseal", suite.ctx, keys, threshold).
		Return(mocks.NewMockSealStatusResponse(false, threshold, threshold), nil).Once()

	// Unseal with all keys at once
	response, err := suite.mockClient.Unseal(suite.ctx, keys, threshold)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), response)
	assert.False(suite.T(), response.Sealed)
	assert.Equal(suite.T(), threshold, response.Progress)
}

// TestMockClientWithCustomFactory tests custom factory behavior
func (suite *ClientMockTestSuite) TestMockClientWithCustomFactory() {
	mockFactory := new(mocks.MockClientFactory)

	// Setup factory to return our mock client
	mockFactory.On("NewClient", "http://vault.test:8200", false, 30*time.Second).
		Return(suite.mockClient, nil).Once()

	// Setup client behavior
	suite.mockClient.On("Close").Return(nil)
	suite.mockClient.On("IsClosed").Return(false)

	// Use factory to create client
	client, err := mockFactory.NewClient("http://vault.test:8200", false, 30*time.Second)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), client)

	// Verify client is working
	assert.False(suite.T(), client.IsClosed())

	// Clean up
	err = client.Close()
	require.NoError(suite.T(), err)

	mockFactory.AssertExpectations(suite.T())
}

// TestMockValidatorSuccess tests successful key validation
func (suite *ClientMockTestSuite) TestMockValidatorSuccess() {
	mockValidator := new(mocks.MockKeyValidator)
	keys := []string{
		"ZGVmYXVsdC11bnNlYWwta2V5LTEtZm9yLXRlc3Rpbmc=",
		"ZGVmYXVsdC11bnNlYWwta2V5LTItZm9yLXRlc3Rpbmc=",
		"ZGVmYXVsdC11bnNlYWwta2V5LTMtZm9yLXRlc3Rpbmc=",
	}

	// Mock successful validation
	mockValidator.On("ValidateKeys", keys, 3).Return(nil).Once()
	for _, key := range keys {
		mockValidator.On("ValidateBase64Key", key).Return(nil).Once()
	}

	// Test validation
	err := mockValidator.ValidateKeys(keys, 3)
	assert.NoError(suite.T(), err)

	for _, key := range keys {
		err := mockValidator.ValidateBase64Key(key)
		assert.NoError(suite.T(), err)
	}

	mockValidator.AssertExpectations(suite.T())
}

// TestMockValidatorFailure tests validation failures
func (suite *ClientMockTestSuite) TestMockValidatorFailure() {
	mockValidator := new(mocks.MockKeyValidator)
	invalidKeys := []string{
		"invalid-key-1",
		"invalid-key-2",
		"invalid-key-3",
	}

	// Mock validation failures
	validationError := fmt.Errorf("validation error: invalid base64 encoding")
	mockValidator.On("ValidateKeys", invalidKeys, 3).Return(validationError).Once()

	for _, key := range invalidKeys {
		mockValidator.On("ValidateBase64Key", key).Return(validationError).Once()
	}

	// Test validation failures
	err := mockValidator.ValidateKeys(invalidKeys, 3)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "invalid base64")

	for _, key := range invalidKeys {
		err := mockValidator.ValidateBase64Key(key)
		assert.Error(suite.T(), err)
		assert.Contains(suite.T(), err.Error(), "invalid base64")
	}

	mockValidator.AssertExpectations(suite.T())
}

// TestMockUnsealStrategy tests unsealing strategy behavior
func (suite *ClientMockTestSuite) TestMockUnsealStrategy() {
	mockStrategy := new(mocks.MockUnsealStrategy)
	keys := []string{
		"ZGVmYXVsdC11bnNlYWwta2V5LTEtZm9yLXRlc3Rpbmc=",
		"ZGVmYXVsdC11bnNlYWwta2V5LTItZm9yLXRlc3Rpbmc=",
		"ZGVmYXVsdC11bnNlYWwta2V5LTMtZm9yLXRlc3Rpbmc=",
	}

	// Mock successful unsealing strategy
	expectedResponse := mocks.NewMockSealStatusResponse(false, 3, 3)
	mockStrategy.On("Unseal", suite.ctx, suite.mockClient, keys, 3).
		Return(expectedResponse, nil).Once()

	// Test strategy
	response, err := mockStrategy.Unseal(suite.ctx, suite.mockClient, keys, 3)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), response)
	assert.False(suite.T(), response.Sealed)
	assert.Equal(suite.T(), 3, response.T)
	assert.Equal(suite.T(), 3, response.Progress)

	mockStrategy.AssertExpectations(suite.T())
}

// TestMockErrorScenarios tests various error scenarios
func (suite *ClientMockTestSuite) TestMockErrorScenarios() {
	tests := []struct {
		name          string
		setupMock     func(*mocks.MockVaultClient)
		operation     func(*mocks.MockVaultClient) error
		expectedError string
	}{
		{
			name: "timeout error",
			setupMock: func(client *mocks.MockVaultClient) {
				client.On("IsSealed", mock.Anything).
					Return(false, fmt.Errorf("operation timed out"))
			},
			operation: func(client *mocks.MockVaultClient) error {
				_, err := client.IsSealed(suite.ctx)
				return err
			},
			expectedError: "operation timed out",
		},
		{
			name: "unauthorized error",
			setupMock: func(client *mocks.MockVaultClient) {
				client.On("HealthCheck", mock.Anything).
					Return(nil, fmt.Errorf("permission denied"))
			},
			operation: func(client *mocks.MockVaultClient) error {
				_, err := client.HealthCheck(suite.ctx)
				return err
			},
			expectedError: "permission denied",
		},
		{
			name: "invalid key error",
			setupMock: func(client *mocks.MockVaultClient) {
				client.On("SubmitSingleKey", mock.Anything, "invalid-key", 0).
					Return(nil, fmt.Errorf("invalid key format"))
			},
			operation: func(client *mocks.MockVaultClient) error {
				_, err := client.SubmitSingleKey(suite.ctx, "invalid-key", 0)
				return err
			},
			expectedError: "invalid key format",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			mockClient := new(mocks.MockVaultClient)
			tt.setupMock(mockClient)

			err := tt.operation(mockClient)
			assert.Error(suite.T(), err)
			assert.Contains(suite.T(), err.Error(), tt.expectedError)

			mockClient.AssertExpectations(suite.T())
		})
	}
}

// TestClientMockTestSuite runs the client mock test suite
func TestClientMockTestSuite(t *testing.T) {
	suite.Run(t, new(ClientMockTestSuite))
}
