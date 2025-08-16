package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/panteparak/vault-autounseal-operator/pkg/core/types"
	"github.com/panteparak/vault-autounseal-operator/pkg/testing/mocks"
)

func TestDefaultUnsealStrategy_Unseal(t *testing.T) {
	mockValidator := &mocks.MockKeyValidator{}
	mockMetrics := &mocks.MockClientMetrics{}
	mockClient := &mocks.MockVaultClient{}

	strategy := NewDefaultUnsealStrategy(mockValidator, mockMetrics)

	keys := []string{"key1", "key2", "key3"}
	threshold := 3

	// Setup mocks
	mockValidator.On("ValidateKeys", keys, threshold).Return(nil)
	mockClient.On("GetSealStatus", mock.Anything).Return(&api.SealStatusResponse{
		Sealed: true,
	}, nil)
	mockClient.On("Unseal", mock.Anything, mock.Anything, mock.Anything).Return(&api.SealStatusResponse{
		Sealed: false,
	}, nil)
	mockMetrics.On("RecordUnsealAttempt", mock.Anything, mock.Anything, mock.Anything)

	ctx := context.Background()
	result, err := strategy.Unseal(ctx, mockClient, keys, threshold)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Sealed)

	mockValidator.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}

func TestDefaultUnsealStrategy_AlreadyUnsealed(t *testing.T) {
	mockValidator := &mocks.MockKeyValidator{}
	mockMetrics := &mocks.MockClientMetrics{}
	mockClient := &mocks.MockVaultClient{}

	strategy := NewDefaultUnsealStrategy(mockValidator, mockMetrics)

	keys := []string{"key1", "key2", "key3"}
	threshold := 3

	// Setup mocks - vault is already unsealed
	mockValidator.On("ValidateKeys", keys, threshold).Return(nil)
	mockClient.On("GetSealStatus", mock.Anything).Return(&api.SealStatusResponse{
		Sealed: false,
	}, nil)
	mockMetrics.On("RecordUnsealAttempt", mock.Anything, true, mock.Anything)

	ctx := context.Background()
	result, err := strategy.Unseal(ctx, mockClient, keys, threshold)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Sealed)

	mockValidator.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}

func TestRetryUnsealStrategy(t *testing.T) {
	mockBaseStrategy := &mocks.MockUnsealStrategy{}
	mockRetryPolicy := &mocks.MockRetryPolicy{}

	retryStrategy := NewRetryUnsealStrategy(mockBaseStrategy, mockRetryPolicy)

	keys := []string{"key1", "key2", "key3"}
	threshold := 3

	// Setup mocks - first attempt fails, second succeeds
	expectedError := types.NewVaultError("test", "endpoint", assert.AnError, true)
	mockBaseStrategy.On("Unseal", mock.Anything, mock.Anything, keys, threshold).Return((*api.SealStatusResponse)(nil), expectedError).Once()
	mockBaseStrategy.On("Unseal", mock.Anything, mock.Anything, keys, threshold).Return(&api.SealStatusResponse{
		Sealed: false,
	}, nil).Once()

	mockRetryPolicy.On("MaxAttempts").Return(3)
	mockRetryPolicy.On("ShouldRetry", expectedError, 0).Return(true)
	mockRetryPolicy.On("NextDelay", 0).Return(100 * time.Millisecond)

	ctx := context.Background()
	result, err := retryStrategy.Unseal(ctx, nil, keys, threshold)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Sealed)

	mockBaseStrategy.AssertExpectations(t)
	mockRetryPolicy.AssertExpectations(t)
}

func TestDefaultRetryPolicy(t *testing.T) {
	policy := NewDefaultRetryPolicy()

	assert.Equal(t, 3, policy.MaxAttempts())

	// Test retry logic
	retryableError := types.NewVaultError("test", "endpoint", assert.AnError, true)
	nonRetryableError := types.NewVaultError("test", "endpoint", assert.AnError, false)

	assert.True(t, policy.ShouldRetry(retryableError, 0))
	assert.False(t, policy.ShouldRetry(nonRetryableError, 0))
	assert.False(t, policy.ShouldRetry(retryableError, 2)) // Max attempts reached

	// Test exponential backoff
	delay1 := policy.NextDelay(0)
	delay2 := policy.NextDelay(1)
	delay3 := policy.NextDelay(2)

	assert.Equal(t, 1*time.Second, delay1)
	assert.Equal(t, 2*time.Second, delay2)
	assert.Equal(t, 4*time.Second, delay3)
}

func TestParallelUnsealStrategy(t *testing.T) {
	mockBaseStrategy := &mocks.MockUnsealStrategy{}
	parallelStrategy := NewParallelUnsealStrategy(mockBaseStrategy, 5)

	keys := []string{"key1", "key2", "key3"}
	threshold := 3

	mockBaseStrategy.On("Unseal", mock.Anything, mock.Anything, keys, threshold).Return(&api.SealStatusResponse{
		Sealed: false,
	}, nil)

	ctx := context.Background()
	result, err := parallelStrategy.Unseal(ctx, nil, keys, threshold)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Sealed)

	mockBaseStrategy.AssertExpectations(t)
}
