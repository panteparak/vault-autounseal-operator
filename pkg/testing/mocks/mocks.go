package mocks

import (
	"context"
	"time"

	"github.com/hashicorp/vault/api"
	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/core/types"
	"github.com/panteparak/vault-autounseal-operator/pkg/vault"
	"github.com/stretchr/testify/mock"
)

const (
	// MockSealThreshold is the default threshold for mock seal responses.
	MockSealThreshold = 5
	// MockServerTimeUTC is a mock timestamp for health responses.
	MockServerTimeUTC = 1234567890
)

// MockVaultClient is a mock implementation of VaultClient
type MockVaultClient struct {
	mock.Mock
}

func (m *MockVaultClient) IsSealed(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

func (m *MockVaultClient) GetSealStatus(ctx context.Context) (*api.SealStatusResponse, error) {
	args := m.Called(ctx)
	if response := args.Get(0); response != nil {
		if sealStatus, ok := response.(*api.SealStatusResponse); ok {
			return sealStatus, args.Error(1)
		}
	}
	return nil, args.Error(1)
}

func (m *MockVaultClient) Unseal(ctx context.Context, keys []string, threshold int) (*api.SealStatusResponse, error) {
	args := m.Called(ctx, keys, threshold)
	if response := args.Get(0); response != nil {
		if sealStatus, ok := response.(*api.SealStatusResponse); ok {
			return sealStatus, args.Error(1)
		}
	}
	return nil, args.Error(1)
}

func (m *MockVaultClient) IsInitialized(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

func (m *MockVaultClient) HealthCheck(ctx context.Context) (*api.HealthResponse, error) {
	args := m.Called(ctx)
	if response := args.Get(0); response != nil {
		if healthResp, ok := response.(*api.HealthResponse); ok {
			return healthResp, args.Error(1)
		}
	}
	return nil, args.Error(1)
}

func (m *MockVaultClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockVaultClient) IsClosed() bool {
	args := m.Called()
	return args.Bool(0)
}

// MockKeyValidator is a mock implementation of KeyValidator
type MockKeyValidator struct {
	mock.Mock
}

func (m *MockKeyValidator) ValidateKeys(keys []string, threshold int) error {
	args := m.Called(keys, threshold)
	return args.Error(0)
}

func (m *MockKeyValidator) ValidateBase64Key(key string) error {
	args := m.Called(key)
	return args.Error(0)
}

// MockUnsealStrategy is a mock implementation of UnsealStrategy
type MockUnsealStrategy struct {
	mock.Mock
}

func (m *MockUnsealStrategy) Unseal(
	ctx context.Context,
	client types.VaultClient,
	keys []string,
	threshold int,
) (*api.SealStatusResponse, error) {
	args := m.Called(ctx, client, keys, threshold)
	if response := args.Get(0); response != nil {
		if sealStatus, ok := response.(*api.SealStatusResponse); ok {
			return sealStatus, args.Error(1)
		}
	}
	return nil, args.Error(1)
}

// MockClientMetrics is a mock implementation of ClientMetrics
type MockClientMetrics struct {
	mock.Mock
}

func (m *MockClientMetrics) RecordUnsealAttempt(endpoint string, success bool, duration time.Duration) {
	m.Called(endpoint, success, duration)
}

func (m *MockClientMetrics) RecordHealthCheck(endpoint string, success bool, duration time.Duration) {
	m.Called(endpoint, success, duration)
}

func (m *MockClientMetrics) RecordSealStatusCheck(endpoint string, success bool, duration time.Duration) {
	m.Called(endpoint, success, duration)
}

// MockRetryPolicy is a mock implementation of RetryPolicy
type MockRetryPolicy struct {
	mock.Mock
}

func (m *MockRetryPolicy) ShouldRetry(err error, attempt int) bool {
	args := m.Called(err, attempt)
	return args.Bool(0)
}

func (m *MockRetryPolicy) NextDelay(attempt int) time.Duration {
	args := m.Called(attempt)
	if duration, ok := args.Get(0).(time.Duration); ok {
		return duration
	}
	return 0
}

func (m *MockRetryPolicy) MaxAttempts() int {
	args := m.Called()
	return args.Int(0)
}

// MockClientFactory is a mock implementation of ClientFactory
type MockClientFactory struct {
	mock.Mock
}

func (m *MockClientFactory) NewClient(
	endpoint string,
	tlsSkipVerify bool,
	timeout time.Duration,
) (vault.VaultClient, error) {
	args := m.Called(endpoint, tlsSkipVerify, timeout)

	client := args.Get(0)
	if client == nil {
		return nil, args.Error(1)
	}

	if vaultClient, ok := client.(vault.VaultClient); ok {
		return vaultClient, args.Error(1)
	}
	return nil, args.Error(1)
}

// MockVaultClientRepository is a mock implementation of VaultClientRepository.
type MockVaultClientRepository struct {
	mock.Mock
}

// GetClient mocks the GetClient method.
func (m *MockVaultClientRepository) GetClient(
	ctx context.Context,
	key string,
	instance *vaultv1.VaultInstance,
) (vault.VaultClient, error) {
	args := m.Called(ctx, key, instance)

	client := args.Get(0)
	if client == nil {
		return nil, args.Error(1)
	}

	if vaultClient, ok := client.(vault.VaultClient); ok {
		return vaultClient, args.Error(1)
	}
	return nil, args.Error(1)
}

// Close mocks the Close method.
func (m *MockVaultClientRepository) Close() error {
	args := m.Called()

	return args.Error(0)
}

// NewMockSealStatusResponse creates a mock SealStatusResponse.
func NewMockSealStatusResponse(sealed bool, progress, total int) *api.SealStatusResponse {
	return &api.SealStatusResponse{
		Type:        "shamir",
		Initialized: true,
		Sealed:      sealed,
		T:           total,
		N:           MockSealThreshold,
		Progress:    progress,
		Nonce:       "test-nonce",
		Version:     "1.15.0",
	}
}

// NewMockHealthResponse creates a mock HealthResponse.
func NewMockHealthResponse(initialized, sealed bool) *api.HealthResponse {
	return &api.HealthResponse{
		Initialized:   initialized,
		Sealed:        sealed,
		Standby:       false,
		ServerTimeUTC: MockServerTimeUTC,
		Version:       "1.15.0",
		ClusterName:   "test-cluster",
		ClusterID:     "test-cluster-id",
	}
}
