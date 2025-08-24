// Package mocks provides mock implementations for testing vault-autounseal-operator
package mocks

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/panteparak/vault-autounseal-operator/pkg/vault"
	"github.com/stretchr/testify/mock"
)

// MockVaultClient is a mock implementation of the VaultClient interface for unit testing
type MockVaultClient struct {
	mock.Mock
}

// Ensure MockVaultClient implements the VaultClient interface
var _ vault.VaultClient = (*MockVaultClient)(nil)

// URL returns the mocked Vault URL
func (m *MockVaultClient) URL() string {
	args := m.Called()
	return args.String(0)
}

// Timeout returns the mocked timeout duration
func (m *MockVaultClient) Timeout() time.Duration {
	args := m.Called()
	return args.Get(0).(time.Duration)
}

// IsClosed returns whether the mocked client is closed
func (m *MockVaultClient) IsClosed() bool {
	args := m.Called()
	return args.Bool(0)
}

// Close closes the mocked client
func (m *MockVaultClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

// IsSealed checks if the mocked vault is sealed
func (m *MockVaultClient) IsSealed(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

// GetSealStatus returns the mocked seal status
func (m *MockVaultClient) GetSealStatus(ctx context.Context) (*api.SealStatusResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.SealStatusResponse), args.Error(1)
}

// Unseal attempts to unseal the mocked vault
func (m *MockVaultClient) Unseal(ctx context.Context, keys []string, threshold int) (*api.SealStatusResponse, error) {
	args := m.Called(ctx, keys, threshold)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.SealStatusResponse), args.Error(1)
}

// SubmitSingleKey submits a single unseal key to the mocked vault
func (m *MockVaultClient) SubmitSingleKey(ctx context.Context, key string, keyIndex int) (*api.SealStatusResponse, error) {
	args := m.Called(ctx, key, keyIndex)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.SealStatusResponse), args.Error(1)
}

// IsInitialized checks if the mocked vault is initialized
func (m *MockVaultClient) IsInitialized(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

// HealthCheck performs a health check on the mocked vault
func (m *MockVaultClient) HealthCheck(ctx context.Context) (*api.HealthResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.HealthResponse), args.Error(1)
}

// MockClientFactory is a mock implementation of the ClientFactory interface
type MockClientFactory struct {
	mock.Mock
}

// Ensure MockClientFactory implements the ClientFactory interface
var _ vault.ClientFactory = (*MockClientFactory)(nil)

// NewClient creates a new mock client
func (m *MockClientFactory) NewClient(url string, tlsSkipVerify bool, timeout time.Duration) (vault.VaultClient, error) {
	args := m.Called(url, tlsSkipVerify, timeout)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(vault.VaultClient), args.Error(1)
}

// MockKeyValidator is a mock implementation of the KeyValidator interface
type MockKeyValidator struct {
	mock.Mock
}

// ValidateKeys validates a set of keys
func (m *MockKeyValidator) ValidateKeys(keys []string, threshold int) error {
	args := m.Called(keys, threshold)
	return args.Error(0)
}

// ValidateBase64Key validates a single base64-encoded key
func (m *MockKeyValidator) ValidateBase64Key(key string) error {
	args := m.Called(key)
	return args.Error(0)
}

// MockUnsealStrategy is a mock implementation of the UnsealStrategy interface
type MockUnsealStrategy struct {
	mock.Mock
}

// Unseal performs the unsealing process using the strategy
func (m *MockUnsealStrategy) Unseal(ctx context.Context, client vault.VaultClient, keys []string, threshold int) (*api.SealStatusResponse, error) {
	args := m.Called(ctx, client, keys, threshold)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.SealStatusResponse), args.Error(1)
}

// Helper functions for creating common mock responses

// NewMockSealStatusResponse creates a mock SealStatusResponse
func NewMockSealStatusResponse(sealed bool, threshold, progress int) *api.SealStatusResponse {
	return &api.SealStatusResponse{
		Sealed:       sealed,
		T:            threshold,
		N:            5, // Default to 5 total keys
		Progress:     progress,
		Version:      "1.19.0",
		Type:         "shamir",
		Initialized:  true,
		StorageType:  "inmem",
		ClusterName:  "vault-cluster-test",
		ClusterID:    "test-cluster-id",
		RecoverySeal: false,
	}
}

// NewMockHealthResponse creates a mock HealthResponse
func NewMockHealthResponse(sealed, initialized bool) *api.HealthResponse {
	return &api.HealthResponse{
		Initialized:                initialized,
		Sealed:                     sealed,
		Standby:                    false,
		PerformanceStandby:         false,
		ReplicationPerformanceMode: "disabled",
		ReplicationDRMode:          "disabled",
		ServerTimeUTC:              time.Now().Unix(),
		Version:                    "1.19.0",
		ClusterName:                "vault-cluster-test",
		ClusterID:                  "test-cluster-id",
	}
}

// MockSetup provides convenient setup methods for common mock scenarios

// SetupHealthyVault configures a mock client for a healthy, unsealed vault
func SetupHealthyVault(mockClient *MockVaultClient) {
	// Only set up expectations for methods that are actually called
	mockClient.On("IsClosed").Return(false)
	mockClient.On("HealthCheck", mock.Anything).Return(NewMockHealthResponse(false, true), nil)
	mockClient.On("IsSealed", mock.Anything).Return(false, nil)
	mockClient.On("IsInitialized", mock.Anything).Return(true, nil)
	mockClient.On("Close").Return(nil)
}

// SetupSealedVault configures a mock client for a healthy, sealed vault
func SetupSealedVault(mockClient *MockVaultClient) {
	// Only set up expectations for methods that are actually called in TestMockClientSealedVault
	mockClient.On("IsSealed", mock.Anything).Return(true, nil)
	mockClient.On("GetSealStatus", mock.Anything).Return(NewMockSealStatusResponse(true, 3, 0), nil)
}

// SetupFailingVault configures a mock client for a vault that returns errors
func SetupFailingVault(mockClient *MockVaultClient, errorMsg string) {
	// Create a simple error since VaultError constructor requires more parameters
	err := fmt.Errorf("vault error: %s", errorMsg)

	// Set up expectations for methods that are actually called in TestMockClientFailingVault
	mockClient.On("IsSealed", mock.Anything).Return(false, err)
	mockClient.On("IsInitialized", mock.Anything).Return(false, err)
	mockClient.On("HealthCheck", mock.Anything).Return(nil, err)
	mockClient.On("GetSealStatus", mock.Anything).Return(nil, err)
}

// SetupUnsealingSequence configures a mock client for a complete unsealing sequence
func SetupUnsealingSequence(mockClient *MockVaultClient, threshold int) {
	// Only set up expectations for methods that are actually called in TestMockClientUnsealingSequence

	// Initial state: sealed
	mockClient.On("IsSealed", mock.Anything).Return(true, nil).Once()

	// Submit keys - the test submits threshold number of keys
	for i := 0; i < threshold; i++ {
		progress := i + 1
		isSealed := progress < threshold
		mockClient.On("SubmitSingleKey", mock.Anything, mock.AnythingOfType("string"), i).
			Return(NewMockSealStatusResponse(isSealed, threshold, progress), nil).Once()
	}

	// Final state check: unsealed
	mockClient.On("IsSealed", mock.Anything).Return(false, nil).Once()
}
