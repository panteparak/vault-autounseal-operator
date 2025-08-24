package metrics

import (
	"testing"
	"time"

	"github.com/panteparak/vault-autounseal-operator/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Use a single metrics instance across all tests to avoid duplicate registration
var globalMetricsForTest *metrics.Metrics

func init() {
	// Create a custom registry for tests to avoid conflicts
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	globalMetricsForTest = metrics.NewMetrics()
}

func TestNewMetrics(t *testing.T) {
	m := globalMetricsForTest

	// Test that all metrics are initialized
	assert.NotNil(t, m.UnsealAttempts, "UnsealAttempts should be initialized")
	assert.NotNil(t, m.UnsealDuration, "UnsealDuration should be initialized")
	assert.NotNil(t, m.SealStatusChecks, "SealStatusChecks should be initialized")
	assert.NotNil(t, m.HealthChecks, "HealthChecks should be initialized")
	assert.NotNil(t, m.ReconciliationTotal, "ReconciliationTotal should be initialized")
	assert.NotNil(t, m.ReconciliationTime, "ReconciliationTime should be initialized")
	assert.NotNil(t, m.VaultInstancesTotal, "VaultInstancesTotal should be initialized")
	assert.NotNil(t, m.VaultInstancesSealed, "VaultInstancesSealed should be initialized")
}

func TestMetricsRecordUnsealAttempt(t *testing.T) {
	m := globalMetricsForTest
	endpoint := "https://vault.example.com:8200"
	duration := 2 * time.Second

	// Test recording success
	m.RecordUnsealAttemptSuccess(endpoint, duration)
	m.RecordUnsealAttemptFailure(endpoint, duration)

	// Test the generic method
	m.RecordUnsealAttempt(endpoint, metrics.ResultSuccess, duration)
	m.RecordUnsealAttempt(endpoint, metrics.ResultFailure, duration)

	// No errors should occur - metrics should be recorded successfully
}

func TestMetricsRecordSealStatusCheck(t *testing.T) {
	m := globalMetricsForTest
	endpoint := "https://vault2.example.com:8200"
	duration := 500 * time.Millisecond

	// Test recording success and failure
	m.RecordSealStatusCheckSuccess(endpoint, duration)
	m.RecordSealStatusCheckFailure(endpoint, duration)

	// Test the generic method
	m.RecordSealStatusCheck(endpoint, metrics.ResultSuccess, duration)
	m.RecordSealStatusCheck(endpoint, metrics.ResultFailure, duration)
}

func TestMetricsRecordHealthCheck(t *testing.T) {
	m := globalMetricsForTest
	endpoint := "https://vault3.example.com:8200"
	duration := 1 * time.Second

	// Test recording success and failure
	m.RecordHealthCheckSuccess(endpoint, duration)
	m.RecordHealthCheckFailure(endpoint, duration)

	// Test the generic method
	m.RecordHealthCheck(endpoint, metrics.ResultSuccess, duration)
	m.RecordHealthCheck(endpoint, metrics.ResultFailure, duration)
}

func TestMetricsRecordReconciliation(t *testing.T) {
	m := globalMetricsForTest
	duration := 3 * time.Second
	resource := "test-vault-config"

	// Test recording success and failure
	m.RecordReconciliationSuccess(duration, resource)
	m.RecordReconciliationFailure(duration, resource)

	// Test the generic method
	m.RecordReconciliation(metrics.ResultSuccess, duration, resource)
	m.RecordReconciliation(metrics.ResultFailure, duration, resource)
}

func TestMetricsSetVaultInstanceCounts(t *testing.T) {
	m := globalMetricsForTest

	// Test setting vault instance counts
	m.SetVaultInstanceCounts(10, 3)
	m.SetVaultInstanceCounts(0, 0)
	m.SetVaultInstanceCounts(5, 5) // All sealed
}

func TestResultConstants(t *testing.T) {
	// Test that result constants are defined correctly
	assert.Equal(t, "success", string(metrics.ResultSuccess))
	assert.Equal(t, "failure", string(metrics.ResultFailure))
}

func TestMetricsNamespace(t *testing.T) {
	// Test that metrics namespace is defined
	assert.Equal(t, "vault_autounseal_operator", metrics.MetricsNamespace)
}

func TestNoOpMetrics(t *testing.T) {
	// Test that NoOpMetrics doesn't panic when called
	noOp := &metrics.NoOpMetrics{}

	endpoint := "https://vault.example.com:8200"
	duration := time.Second
	resource := "test-resource"

	// All of these should be no-ops and not panic
	require.NotPanics(t, func() {
		noOp.RecordUnsealAttempt(endpoint, metrics.ResultSuccess, duration)
	})

	require.NotPanics(t, func() {
		noOp.RecordSealStatusCheck(endpoint, metrics.ResultSuccess, duration)
	})

	require.NotPanics(t, func() {
		noOp.RecordHealthCheck(endpoint, metrics.ResultSuccess, duration)
	})

	require.NotPanics(t, func() {
		noOp.RecordReconciliation(metrics.ResultSuccess, duration, resource)
	})

	require.NotPanics(t, func() {
		noOp.SetVaultInstanceCounts(10, 3)
	})
}

func TestMetricsComprehensive(t *testing.T) {
	// Comprehensive test simulating operator behavior
	m := globalMetricsForTest

	// Simulate multiple vault instances
	endpoints := []string{
		"https://vault-comp1.example.com:8200",
		"https://vault-comp2.example.com:8200",
		"https://vault-comp3.example.com:8200",
	}

	// Record various metrics
	for i, endpoint := range endpoints {
		duration := time.Duration(i+1) * time.Second

		// Seal status check
		if i%2 == 0 {
			m.RecordSealStatusCheckSuccess(endpoint, duration)
		} else {
			m.RecordSealStatusCheckFailure(endpoint, duration)
		}

		// Health check
		m.RecordHealthCheckSuccess(endpoint, duration)

		// Unseal attempt
		if i == 0 {
			m.RecordUnsealAttemptSuccess(endpoint, duration)
		} else {
			m.RecordUnsealAttemptFailure(endpoint, duration)
		}
	}

	// Reconciliation metrics
	m.RecordReconciliationSuccess(2*time.Second, "vault-config-1")
	m.RecordReconciliationFailure(3*time.Second, "vault-config-2")

	// Set vault instance counts
	m.SetVaultInstanceCounts(3, 2) // 3 total, 2 sealed
}

func BenchmarkMetricsRecording(b *testing.B) {
	m := globalMetricsForTest
	endpoint := "https://vault-bench.example.com:8200"
	duration := time.Second

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordUnsealAttemptSuccess(endpoint, duration)
		m.RecordSealStatusCheckSuccess(endpoint, duration)
		m.RecordHealthCheckSuccess(endpoint, duration)
		m.RecordReconciliationSuccess(duration, "test-resource")
		m.SetVaultInstanceCounts(10, 5)
	}
}
