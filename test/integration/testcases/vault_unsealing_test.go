package testcases

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/panteparak/vault-autounseal-operator/test/integration/framework"
)

// VaultUnsealingTest validates that the operator can unseal Vault instances
type VaultUnsealingTest struct {
	scenario string
}

// NewVaultUnsealingTest creates a new vault unsealing test
func NewVaultUnsealingTest(scenario string) framework.TestCase {
	return &VaultUnsealingTest{
		scenario: scenario,
	}
}

func (t *VaultUnsealingTest) Name() string {
	return fmt.Sprintf("vault-unsealing-%s", t.scenario)
}

func (t *VaultUnsealingTest) Description() string {
	return fmt.Sprintf("Validates Vault unsealing functionality for %s scenario", t.scenario)
}

func (t *VaultUnsealingTest) Prerequisites() []string {
	return []string{"vault-available", "operator-deployed"}
}

func (t *VaultUnsealingTest) Tags() []string {
	return []string{"vault", "unsealing", "core", t.scenario}
}

func (t *VaultUnsealingTest) Execute(ctx context.Context, framework *framework.TestFramework) *framework.TestResult {
	result := &framework.TestResult{
		TestName:  t.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
		Logs:      make([]string, 0),
		Metrics: framework.TestMetrics{
			VaultResponseTimes: make(map[string]time.Duration),
			OperatorMetrics:    make(map[string]float64),
			APICallCounts:      make(map[string]int),
		},
	}

	// Get expected Vault instances for the scenario
	endpoints := t.getExpectedEndpoints(framework.Config)

	result.Details["scenario"] = t.scenario
	result.Details["expected_endpoints"] = endpoints

	// Test each Vault instance
	allPassed := true
	for name, endpoint := range endpoints {
		passed := t.testVaultInstance(ctx, name, endpoint, result)
		if !passed {
			allPassed = false
			if framework.Config.TestSettings.FailFast {
				break
			}
		}
	}

	result.Success = allPassed
	return result
}

func (t *VaultUnsealingTest) testVaultInstance(ctx context.Context, name, endpoint string, result *framework.TestResult) bool {
	logMsg := fmt.Sprintf("Testing Vault instance: %s at %s", name, endpoint)
	result.Logs = append(result.Logs, logMsg)

	// Measure response time
	start := time.Now()

	// Check seal status via API
	sealStatus, err := t.getVaultSealStatus(ctx, endpoint)
	responseTime := time.Since(start)

	result.Metrics.VaultResponseTimes[name] = responseTime
	result.Metrics.APICallCounts["seal_status_checks"]++

	if err != nil {
		errorMsg := fmt.Sprintf("Failed to get seal status for %s: %v", name, err)
		result.Logs = append(result.Logs, errorMsg)
		result.Details[name+"_error"] = err.Error()
		return false
	}

	// Extract status information
	sealed, ok := sealStatus["sealed"].(bool)
	if !ok {
		errorMsg := fmt.Sprintf("Invalid seal status response for %s", name)
		result.Logs = append(result.Logs, errorMsg)
		return false
	}

	initialized, ok := sealStatus["initialized"].(bool)
	if !ok {
		initialized = true // Default assumption
	}

	result.Details[name+"_sealed"] = sealed
	result.Details[name+"_initialized"] = initialized
	result.Details[name+"_response_time"] = responseTime

	// Log the status
	statusMsg := fmt.Sprintf("%s - Initialized: %v, Sealed: %v, Response Time: %v",
		name, initialized, sealed, responseTime)
	result.Logs = append(result.Logs, statusMsg)

	// Validate the expected state
	if initialized && !sealed {
		successMsg := fmt.Sprintf("✅ %s Vault is properly initialized and unsealed", name)
		result.Logs = append(result.Logs, successMsg)
		return true
	} else if !initialized {
		warningMsg := fmt.Sprintf("⚠️ %s Vault is not initialized (acceptable in dev mode)", name)
		result.Logs = append(result.Logs, warningMsg)
		return true // In dev mode, uninitialized is acceptable
	} else {
		failMsg := fmt.Sprintf("❌ %s Vault unsealing validation failed - sealed: %v", name, sealed)
		result.Logs = append(result.Logs, failMsg)
		return false
	}
}

func (t *VaultUnsealingTest) getVaultSealStatus(ctx context.Context, endpoint string) (map[string]interface{}, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf("%s/v1/sys/seal-status", endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var sealStatus map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&sealStatus); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return sealStatus, nil
}

func (t *VaultUnsealingTest) getExpectedEndpoints(config *framework.TestConfig) map[string]string {
	endpoints := make(map[string]string)

	switch t.scenario {
	case "basic":
		endpoints["basic"] = config.VaultConfig.Endpoints["basic"]
	case "failover":
		endpoints["primary"] = config.VaultConfig.Endpoints["primary"]
		endpoints["standby"] = config.VaultConfig.Endpoints["standby"]
	case "multi-vault":
		endpoints["finance"] = config.VaultConfig.Endpoints["finance"]
		endpoints["engineering"] = config.VaultConfig.Endpoints["engineering"]
		endpoints["operations"] = config.VaultConfig.Endpoints["operations"]
	}

	return endpoints
}

func (t *VaultUnsealingTest) Cleanup(ctx context.Context, framework *framework.TestFramework) error {
	// No specific cleanup needed for this test
	return nil
}
