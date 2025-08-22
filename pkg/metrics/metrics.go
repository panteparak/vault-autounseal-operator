package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Result represents the outcome of an operation.
type Result string

const (
	// MetricsNamespace is the namespace for all metrics.
	MetricsNamespace = "vault_autounseal_operator"

	// Result constants for consistent labeling.
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
)

// Metrics holds all prometheus metrics for the operator.
type Metrics struct {
	UnsealAttempts       *prometheus.CounterVec
	UnsealDuration       *prometheus.HistogramVec
	SealStatusChecks     *prometheus.CounterVec
	HealthChecks         *prometheus.CounterVec
	ReconciliationTotal  *prometheus.CounterVec
	ReconciliationTime   *prometheus.HistogramVec
	VaultInstancesTotal  prometheus.Gauge
	VaultInstancesSealed prometheus.Gauge
}

// NewMetrics creates a new metrics collector.
func NewMetrics() *Metrics {
	m := &Metrics{}
	m.initCounterMetrics()
	m.initHistogramMetrics()
	m.initGaugeMetrics()
	return m
}

// initCounterMetrics initializes counter metrics.
func (m *Metrics) initCounterMetrics() {
	m.UnsealAttempts = newCounterVec("unseal_attempts_total",
		"Total number of vault unseal attempts", []string{"endpoint", "result"})
	m.SealStatusChecks = newCounterVec("seal_status_checks_total",
		"Total number of seal status checks", []string{"endpoint", "result"})
	m.HealthChecks = newCounterVec("health_checks_total",
		"Total number of health checks", []string{"endpoint", "result"})
	m.ReconciliationTotal = newCounterVec("reconciliation_total",
		"Total number of reconciliations", []string{"result"})
}

// initHistogramMetrics initializes histogram metrics.
func (m *Metrics) initHistogramMetrics() {
	m.UnsealDuration = newHistogramVec("unseal_duration_seconds",
		"Duration of vault unseal operations", []string{"endpoint"})
	m.ReconciliationTime = newHistogramVec("reconciliation_duration_seconds",
		"Duration of reconciliation operations", []string{"resource"})
}

// initGaugeMetrics initializes gauge metrics.
func (m *Metrics) initGaugeMetrics() {
	m.VaultInstancesTotal = newGauge("vault_instances",
		"Total number of vault instances being managed")
	m.VaultInstancesSealed = newGauge("vault_instances_sealed",
		"Number of vault instances that are currently sealed")
}

// Helper functions for creating metrics.
func newCounterVec(name, help string, labels []string) *prometheus.CounterVec {
	return promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: MetricsNamespace,
		Name:      name,
		Help:      help,
	}, labels)
}

func newHistogramVec(name, help string, labels []string) *prometheus.HistogramVec {
	return promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: MetricsNamespace,
		Name:      name,
		Help:      help,
		Buckets:   prometheus.DefBuckets,
	}, labels)
}

func newGauge(name, help string) prometheus.Gauge {
	return promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: MetricsNamespace,
		Name:      name,
		Help:      help,
	})
}

// RecordUnsealAttempt records an unseal attempt.
func (m *Metrics) RecordUnsealAttempt(endpoint string, result Result, duration time.Duration) {
	m.UnsealAttempts.WithLabelValues(endpoint, string(result)).Inc()
	m.UnsealDuration.WithLabelValues(endpoint).Observe(duration.Seconds())
}

// RecordUnsealAttemptSuccess records a successful unseal attempt.
func (m *Metrics) RecordUnsealAttemptSuccess(endpoint string, duration time.Duration) {
	m.RecordUnsealAttempt(endpoint, ResultSuccess, duration)
}

// RecordUnsealAttemptFailure records a failed unseal attempt.
func (m *Metrics) RecordUnsealAttemptFailure(endpoint string, duration time.Duration) {
	m.RecordUnsealAttempt(endpoint, ResultFailure, duration)
}

// RecordSealStatusCheck records a seal status check.
func (m *Metrics) RecordSealStatusCheck(endpoint string, result Result, _ time.Duration) {
	m.SealStatusChecks.WithLabelValues(endpoint, string(result)).Inc()
}

// RecordSealStatusCheckSuccess records a successful seal status check.
func (m *Metrics) RecordSealStatusCheckSuccess(endpoint string, duration time.Duration) {
	m.RecordSealStatusCheck(endpoint, ResultSuccess, duration)
}

// RecordSealStatusCheckFailure records a failed seal status check.
func (m *Metrics) RecordSealStatusCheckFailure(endpoint string, duration time.Duration) {
	m.RecordSealStatusCheck(endpoint, ResultFailure, duration)
}

// RecordHealthCheck records a health check.
func (m *Metrics) RecordHealthCheck(endpoint string, result Result, _ time.Duration) {
	m.HealthChecks.WithLabelValues(endpoint, string(result)).Inc()
}

// RecordHealthCheckSuccess records a successful health check.
func (m *Metrics) RecordHealthCheckSuccess(endpoint string, duration time.Duration) {
	m.RecordHealthCheck(endpoint, ResultSuccess, duration)
}

// RecordHealthCheckFailure records a failed health check.
func (m *Metrics) RecordHealthCheckFailure(endpoint string, duration time.Duration) {
	m.RecordHealthCheck(endpoint, ResultFailure, duration)
}

// RecordReconciliation records a reconciliation.
func (m *Metrics) RecordReconciliation(result Result, duration time.Duration, resource string) {
	m.ReconciliationTotal.WithLabelValues(string(result)).Inc()
	m.ReconciliationTime.WithLabelValues(resource).Observe(duration.Seconds())
}

// RecordReconciliationSuccess records a successful reconciliation.
func (m *Metrics) RecordReconciliationSuccess(duration time.Duration, resource string) {
	m.RecordReconciliation(ResultSuccess, duration, resource)
}

// RecordReconciliationFailure records a failed reconciliation.
func (m *Metrics) RecordReconciliationFailure(duration time.Duration, resource string) {
	m.RecordReconciliation(ResultFailure, duration, resource)
}

// SetVaultInstanceCounts sets the vault instance counts.
func (m *Metrics) SetVaultInstanceCounts(total, sealed int) {
	m.VaultInstancesTotal.Set(float64(total))
	m.VaultInstancesSealed.Set(float64(sealed))
}

// NoOpMetrics provides a no-op implementation for testing.
type NoOpMetrics struct{}

// RecordUnsealAttempt does nothing.
func (m *NoOpMetrics) RecordUnsealAttempt(_ string, _ Result, _ time.Duration) {}

// RecordSealStatusCheck does nothing.
func (m *NoOpMetrics) RecordSealStatusCheck(_ string, _ Result, _ time.Duration) {}

// RecordHealthCheck does nothing.
func (m *NoOpMetrics) RecordHealthCheck(_ string, _ Result, _ time.Duration) {}

// RecordReconciliation does nothing.
func (m *NoOpMetrics) RecordReconciliation(_ Result, _ time.Duration, _ string) {}

// SetVaultInstanceCounts does nothing.
func (m *NoOpMetrics) SetVaultInstanceCounts(_, _ int) {}
