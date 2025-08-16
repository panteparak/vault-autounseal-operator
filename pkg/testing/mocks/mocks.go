package mocks

import (
	"context"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/mock"

	"github.com/panteparak/vault-autounseal-operator/pkg/core/types"
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
	return args.Get(0).(*api.SealStatusResponse), args.Error(1)
}

func (m *MockVaultClient) Unseal(ctx context.Context, keys []string, threshold int) (*api.SealStatusResponse, error) {
	args := m.Called(ctx, keys, threshold)
	return args.Get(0).(*api.SealStatusResponse), args.Error(1)
}

func (m *MockVaultClient) IsInitialized(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

func (m *MockVaultClient) HealthCheck(ctx context.Context) (*api.HealthResponse, error) {
	args := m.Called(ctx)
	return args.Get(0).(*api.HealthResponse), args.Error(1)
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

func (m *MockUnsealStrategy) Unseal(ctx context.Context, client types.VaultClient, keys []string, threshold int) (*api.SealStatusResponse, error) {
	args := m.Called(ctx, client, keys, threshold)
	return args.Get(0).(*api.SealStatusResponse), args.Error(1)
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
	return args.Get(0).(time.Duration)
}

func (m *MockRetryPolicy) MaxAttempts() int {
	args := m.Called()
	return args.Int(0)
}

// MockClientFactory is a mock implementation of ClientFactory
type MockClientFactory struct {
	mock.Mock
}

func (m *MockClientFactory) NewClient(endpoint string, tlsSkipVerify bool, timeout time.Duration) (types.VaultClient, error) {
	args := m.Called(endpoint, tlsSkipVerify, timeout)
	return args.Get(0).(types.VaultClient), args.Error(1)
}
