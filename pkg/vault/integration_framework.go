package vault

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// IntegrationTestConfig defines configuration for fast-failing integration tests
type IntegrationTestConfig struct {
	// Fail-fast timeouts
	QuickTimeout     time.Duration // For health checks (default: 2s)
	OperationTimeout time.Duration // For operations (default: 5s)
	MaxTotalTime     time.Duration // Maximum test duration (default: 30s)

	// Circuit breaker settings
	FailureThreshold int           // Number of failures before breaking (default: 3)
	SuccessThreshold int           // Successes needed to reset (default: 2)
	CooldownPeriod   time.Duration // Wait before retry (default: 1s)

	// Health check configuration
	HealthCheckInterval time.Duration // How often to check (default: 500ms)
	MaxUnhealthyTime    time.Duration // Max time to wait for health (default: 10s)

	// Parallel execution
	MaxConcurrency int // Max parallel operations (default: 5)
}

// DefaultIntegrationConfig returns sensible defaults for fast integration testing
func DefaultIntegrationConfig() *IntegrationTestConfig {
	return &IntegrationTestConfig{
		QuickTimeout:        2 * time.Second,
		OperationTimeout:    5 * time.Second,
		MaxTotalTime:        30 * time.Second,
		FailureThreshold:    3,
		SuccessThreshold:    2,
		CooldownPeriod:      1 * time.Second,
		HealthCheckInterval: 500 * time.Millisecond,
		MaxUnhealthyTime:    10 * time.Second,
		MaxConcurrency:      5,
	}
}

// CircuitBreaker implements fail-fast circuit breaker pattern for testing
type CircuitBreaker struct {
	name             string
	failures         int
	successes        int
	lastFailureTime  time.Time
	state            CircuitState
	failureThreshold int
	successThreshold int
	cooldownPeriod   time.Duration
	mutex            sync.RWMutex
}

type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Failing fast
	CircuitHalfOpen                     // Testing recovery
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "CLOSED"
	case CircuitOpen:
		return "OPEN"
	case CircuitHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// NewCircuitBreaker creates a circuit breaker for integration testing
func NewCircuitBreaker(name string, config *IntegrationTestConfig) *CircuitBreaker {
	return &CircuitBreaker{
		name:             name,
		failureThreshold: config.FailureThreshold,
		successThreshold: config.SuccessThreshold,
		cooldownPeriod:   config.CooldownPeriod,
		state:            CircuitClosed,
	}
}

// Execute runs an operation through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, operation func() error) error {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Check if circuit is open and we should fail fast
	if cb.state == CircuitOpen {
		if time.Since(cb.lastFailureTime) < cb.cooldownPeriod {
			return fmt.Errorf("circuit breaker %s is OPEN - failing fast", cb.name)
		}
		// Try to transition to half-open
		cb.state = CircuitHalfOpen
	}

	// Execute the operation
	err := operation()

	if err != nil {
		cb.recordFailure()
		return fmt.Errorf("operation failed: %w", err)
	}

	cb.recordSuccess()
	return nil
}

func (cb *CircuitBreaker) recordFailure() {
	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.failureThreshold {
		cb.state = CircuitOpen
		cb.successes = 0 // Reset success counter
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.successes++

	if cb.state == CircuitHalfOpen && cb.successes >= cb.successThreshold {
		cb.state = CircuitClosed
		cb.failures = 0 // Reset failure counter
	}
}

// GetState returns current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetStats returns circuit breaker statistics
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	return map[string]interface{}{
		"name":        cb.name,
		"state":       cb.state.String(),
		"failures":    cb.failures,
		"successes":   cb.successes,
		"lastFailure": cb.lastFailureTime,
	}
}

// HealthChecker manages health checking for integration tests
type HealthChecker struct {
	config  *IntegrationTestConfig
	clients map[string]VaultClient
	healthy map[string]bool
	mutex   sync.RWMutex
}

// NewHealthChecker creates a health checker for integration testing
func NewHealthChecker(config *IntegrationTestConfig) *HealthChecker {
	return &HealthChecker{
		config:  config,
		clients: make(map[string]VaultClient),
		healthy: make(map[string]bool),
	}
}

// RegisterClient adds a client to health monitoring
func (hc *HealthChecker) RegisterClient(name string, client VaultClient) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	hc.clients[name] = client
	hc.healthy[name] = false // Start as unhealthy until proven otherwise
}

// CheckHealth performs health checks on all registered clients
func (hc *HealthChecker) CheckHealth(ctx context.Context) error {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	healthyCount := 0
	totalCount := len(hc.clients)

	for name, client := range hc.clients {
		quickCtx, cancel := context.WithTimeout(ctx, hc.config.QuickTimeout)
		_, err := client.HealthCheck(quickCtx)
		cancel()

		if err == nil {
			hc.healthy[name] = true
			healthyCount++
		} else {
			hc.healthy[name] = false
		}
	}

	if healthyCount == 0 && totalCount > 0 {
		return fmt.Errorf("no healthy clients found (%d total)", totalCount)
	}

	return nil
}

// WaitForHealthy waits for at least one client to become healthy
func (hc *HealthChecker) WaitForHealthy(ctx context.Context) error {
	deadline := time.Now().Add(hc.config.MaxUnhealthyTime)
	ticker := time.NewTicker(hc.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for healthy clients after %v", hc.config.MaxUnhealthyTime)
			}

			if err := hc.CheckHealth(ctx); err == nil {
				return nil // At least one client is healthy
			}
		}
	}
}

// GetHealthyClients returns list of currently healthy client names
func (hc *HealthChecker) GetHealthyClients() []string {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	var healthy []string
	for name, isHealthy := range hc.healthy {
		if isHealthy {
			healthy = append(healthy, name)
		}
	}
	return healthy
}

// IntegrationTestRunner orchestrates fast-failing integration tests
type IntegrationTestRunner struct {
	config         *IntegrationTestConfig
	healthChecker  *HealthChecker
	circuitBreaker *CircuitBreaker
	semaphore      chan struct{} // For limiting concurrency
}

// NewIntegrationTestRunner creates a new integration test runner
func NewIntegrationTestRunner(config *IntegrationTestConfig) *IntegrationTestRunner {
	if config == nil {
		config = DefaultIntegrationConfig()
	}

	return &IntegrationTestRunner{
		config:         config,
		healthChecker:  NewHealthChecker(config),
		circuitBreaker: NewCircuitBreaker("integration-tests", config),
		semaphore:      make(chan struct{}, config.MaxConcurrency),
	}
}

// RegisterClient adds a client for testing
func (itr *IntegrationTestRunner) RegisterClient(name string, client VaultClient) {
	itr.healthChecker.RegisterClient(name, client)
}

// RunTest executes a test with fail-fast behavior
func (itr *IntegrationTestRunner) RunTest(ctx context.Context, testName string, testFunc func(context.Context) error) error {
	// Apply overall timeout
	testCtx, cancel := context.WithTimeout(ctx, itr.config.MaxTotalTime)
	defer cancel()

	// Wait for at least one healthy client before starting
	if err := itr.healthChecker.WaitForHealthy(testCtx); err != nil {
		return fmt.Errorf("test %s failed - no healthy clients: %w", testName, err)
	}

	// Acquire concurrency limit
	select {
	case itr.semaphore <- struct{}{}:
		defer func() { <-itr.semaphore }()
	case <-testCtx.Done():
		return fmt.Errorf("test %s failed - timeout acquiring concurrency slot", testName)
	}

	// Execute test through circuit breaker
	return itr.circuitBreaker.Execute(testCtx, func() error {
		// Apply operation timeout
		opCtx, opCancel := context.WithTimeout(testCtx, itr.config.OperationTimeout)
		defer opCancel()

		return testFunc(opCtx)
	})
}

// GetStats returns comprehensive test runner statistics
func (itr *IntegrationTestRunner) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"config":         itr.config,
		"healthyClients": itr.healthChecker.GetHealthyClients(),
		"circuitBreaker": itr.circuitBreaker.GetStats(),
	}
}

// TestScenario represents a specific integration test scenario
type TestScenario struct {
	Name        string
	Description string
	Setup       func(context.Context) error
	Execute     func(context.Context) error
	Cleanup     func(context.Context) error
	ExpectError bool
	Timeout     time.Duration
}

// RunScenarios executes multiple test scenarios with fail-fast behavior
func (itr *IntegrationTestRunner) RunScenarios(ctx context.Context, scenarios []TestScenario) error {
	for _, scenario := range scenarios {
		// Check if circuit breaker is open
		if itr.circuitBreaker.GetState() == CircuitOpen {
			return fmt.Errorf("circuit breaker is OPEN - failing fast on scenario: %s", scenario.Name)
		}

		scenarioCtx := ctx
		if scenario.Timeout > 0 {
			var cancel context.CancelFunc
			scenarioCtx, cancel = context.WithTimeout(ctx, scenario.Timeout)
			defer cancel()
		}

		// Setup
		if scenario.Setup != nil {
			if err := scenario.Setup(scenarioCtx); err != nil {
				return fmt.Errorf("scenario %s setup failed: %w", scenario.Name, err)
			}
		}

		// Execute main test
		err := itr.RunTest(scenarioCtx, scenario.Name, scenario.Execute)

		// Cleanup (always run, even on failure)
		if scenario.Cleanup != nil {
			if cleanupErr := scenario.Cleanup(scenarioCtx); cleanupErr != nil {
				if err == nil {
					err = fmt.Errorf("scenario %s cleanup failed: %w", scenario.Name, cleanupErr)
				}
				// If both test and cleanup failed, combine errors
			}
		}

		// Check if error matches expectation
		if scenario.ExpectError && err == nil {
			return fmt.Errorf("scenario %s expected error but succeeded", scenario.Name)
		}
		if !scenario.ExpectError && err != nil {
			return fmt.Errorf("scenario %s failed: %w", scenario.Name, err)
		}
	}

	return nil
}
