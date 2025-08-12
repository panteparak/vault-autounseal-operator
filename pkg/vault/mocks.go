package vault

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
)

// MockVaultClient implements VaultClient for testing
type MockVaultClient struct {
	mu              sync.RWMutex
	sealed          bool
	initialized     bool
	healthy         bool
	unsealProgress  int
	unsealThreshold int
	submittedKeys   []string
	failHealthCheck bool
	failSealStatus  bool
	failUnseal      bool
	failInitialized bool
	responseDelay   time.Duration
	callCounts      map[string]int
	lastError       error
	sealStatusResp  *api.SealStatusResponse
	healthResp      *api.HealthResponse
}

// NewMockVaultClient creates a new mock vault client
func NewMockVaultClient() *MockVaultClient {
	return &MockVaultClient{
		sealed:          true,
		initialized:     true,
		healthy:         true,
		unsealThreshold: 3,
		callCounts:      make(map[string]int),
		sealStatusResp: &api.SealStatusResponse{
			Sealed:   true,
			Progress: 0,
			T:        3, // Threshold field name in vault API
			Version:  "1.15.0",
		},
		healthResp: &api.HealthResponse{
			Initialized:   true,
			Sealed:        true,
			Standby:       false,
			ServerTimeUTC: time.Now().Unix(),
		},
	}
}

// IsSealed implements VaultClient
func (m *MockVaultClient) IsSealed(ctx context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCounts["IsSealed"]++

	if m.responseDelay > 0 {
		time.Sleep(m.responseDelay)
	}

	if m.failSealStatus {
		m.lastError = fmt.Errorf("mock seal status error")
		return true, m.lastError
	}

	return m.sealed, nil
}

// GetSealStatus implements VaultClient
func (m *MockVaultClient) GetSealStatus(ctx context.Context) (*api.SealStatusResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCounts["GetSealStatus"]++

	if m.responseDelay > 0 {
		time.Sleep(m.responseDelay)
	}

	if m.failSealStatus {
		m.lastError = fmt.Errorf("mock seal status error")
		return nil, m.lastError
	}

	// Update response with current state
	m.sealStatusResp.Sealed = m.sealed
	m.sealStatusResp.Progress = m.unsealProgress

	return m.sealStatusResp, nil
}

// Unseal implements VaultClient
func (m *MockVaultClient) Unseal(ctx context.Context, keys []string, threshold int) (*api.SealStatusResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCounts["Unseal"]++

	if m.responseDelay > 0 {
		time.Sleep(m.responseDelay)
	}

	if m.failUnseal {
		m.lastError = fmt.Errorf("mock unseal error")
		return nil, m.lastError
	}

	// Simulate unseal process
	m.submittedKeys = append(m.submittedKeys, keys...)
	if len(m.submittedKeys) >= threshold {
		m.sealed = false
		m.unsealProgress = threshold
	} else {
		m.unsealProgress = len(m.submittedKeys)
	}

	m.sealStatusResp.Sealed = m.sealed
	m.sealStatusResp.Progress = m.unsealProgress

	return m.sealStatusResp, nil
}

// IsInitialized implements VaultClient
func (m *MockVaultClient) IsInitialized(ctx context.Context) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.callCounts["IsInitialized"]++

	if m.responseDelay > 0 {
		time.Sleep(m.responseDelay)
	}

	if m.failInitialized {
		m.lastError = fmt.Errorf("mock initialized check error")
		return false, m.lastError
	}

	return m.initialized, nil
}

// HealthCheck implements VaultClient
func (m *MockVaultClient) HealthCheck(ctx context.Context) (*api.HealthResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.callCounts["HealthCheck"]++

	if m.responseDelay > 0 {
		time.Sleep(m.responseDelay)
	}

	if m.failHealthCheck {
		m.lastError = fmt.Errorf("mock health check error")
		return nil, m.lastError
	}

	m.healthResp.Sealed = m.sealed
	return m.healthResp, nil
}

// Close implements VaultClient
func (m *MockVaultClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCounts["Close"]++
	return nil
}

// Mock control methods
func (m *MockVaultClient) SetSealed(sealed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sealed = sealed
	if !sealed {
		m.unsealProgress = m.unsealThreshold
	} else {
		m.unsealProgress = 0
		m.submittedKeys = nil
	}
}

func (m *MockVaultClient) SetHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = healthy
}

func (m *MockVaultClient) SetFailHealthCheck(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failHealthCheck = fail
}

func (m *MockVaultClient) SetFailSealStatus(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failSealStatus = fail
}

func (m *MockVaultClient) SetFailUnseal(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failUnseal = fail
}

func (m *MockVaultClient) SetResponseDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseDelay = delay
}

func (m *MockVaultClient) GetCallCount(method string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCounts[method]
}

func (m *MockVaultClient) GetSubmittedKeys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, len(m.submittedKeys))
	copy(keys, m.submittedKeys)
	return keys
}

func (m *MockVaultClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sealed = true
	m.healthy = true
	m.unsealProgress = 0
	m.submittedKeys = nil
	m.failHealthCheck = false
	m.failSealStatus = false
	m.failUnseal = false
	m.failInitialized = false
	m.responseDelay = 0
	m.callCounts = make(map[string]int)
	m.lastError = nil
}

// MockClientMetrics implements ClientMetrics for testing
type MockClientMetrics struct {
	mu               sync.RWMutex
	unsealAttempts   []UnsealAttemptMetric
	healthChecks     []HealthCheckMetric
	sealStatusChecks []SealStatusCheckMetric
}

type UnsealAttemptMetric struct {
	Endpoint string
	Success  bool
	Duration time.Duration
}

type HealthCheckMetric struct {
	Endpoint string
	Success  bool
	Duration time.Duration
}

type SealStatusCheckMetric struct {
	Endpoint string
	Success  bool
	Duration time.Duration
}

// NewMockClientMetrics creates a new mock metrics collector
func NewMockClientMetrics() *MockClientMetrics {
	return &MockClientMetrics{}
}

// RecordUnsealAttempt implements ClientMetrics
func (m *MockClientMetrics) RecordUnsealAttempt(endpoint string, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unsealAttempts = append(m.unsealAttempts, UnsealAttemptMetric{
		Endpoint: endpoint,
		Success:  success,
		Duration: duration,
	})
}

// RecordHealthCheck implements ClientMetrics
func (m *MockClientMetrics) RecordHealthCheck(endpoint string, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthChecks = append(m.healthChecks, HealthCheckMetric{
		Endpoint: endpoint,
		Success:  success,
		Duration: duration,
	})
}

// RecordSealStatusCheck implements ClientMetrics
func (m *MockClientMetrics) RecordSealStatusCheck(endpoint string, success bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sealStatusChecks = append(m.sealStatusChecks, SealStatusCheckMetric{
		Endpoint: endpoint,
		Success:  success,
		Duration: duration,
	})
}

// Test helper methods
func (m *MockClientMetrics) GetUnsealAttempts() []UnsealAttemptMetric {
	m.mu.RLock()
	defer m.mu.RUnlock()
	attempts := make([]UnsealAttemptMetric, len(m.unsealAttempts))
	copy(attempts, m.unsealAttempts)
	return attempts
}

func (m *MockClientMetrics) GetHealthChecks() []HealthCheckMetric {
	m.mu.RLock()
	defer m.mu.RUnlock()
	checks := make([]HealthCheckMetric, len(m.healthChecks))
	copy(checks, m.healthChecks)
	return checks
}

func (m *MockClientMetrics) GetSealStatusChecks() []SealStatusCheckMetric {
	m.mu.RLock()
	defer m.mu.RUnlock()
	checks := make([]SealStatusCheckMetric, len(m.sealStatusChecks))
	copy(checks, m.sealStatusChecks)
	return checks
}

func (m *MockClientMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unsealAttempts = nil
	m.healthChecks = nil
	m.sealStatusChecks = nil
}

// MockClientFactory implements ClientFactory for testing
type MockClientFactory struct {
	mu      sync.RWMutex
	clients map[string]*MockVaultClient
	failNew bool
}

// NewMockClientFactory creates a new mock client factory
func NewMockClientFactory() *MockClientFactory {
	return &MockClientFactory{
		clients: make(map[string]*MockVaultClient),
	}
}

// NewClient implements ClientFactory
func (f *MockClientFactory) NewClient(endpoint string, tlsSkipVerify bool, timeout time.Duration) (VaultClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failNew {
		return nil, fmt.Errorf("mock client factory error")
	}

	client := NewMockVaultClient()
	f.clients[endpoint] = client
	return client, nil
}

// GetClient returns the mock client for testing
func (f *MockClientFactory) GetClient(endpoint string) *MockVaultClient {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.clients[endpoint]
}

// SetFailNew makes the factory fail on new client creation
func (f *MockClientFactory) SetFailNew(fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNew = fail
}

// Reset clears all clients
func (f *MockClientFactory) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clients = make(map[string]*MockVaultClient)
	f.failNew = false
}
