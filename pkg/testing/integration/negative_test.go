//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/panteparak/vault-autounseal-operator/pkg/unsealing/client"
)

// NegativeTestSuite tests failure scenarios and error handling
type NegativeTestSuite struct {
	suite.Suite
	ctx context.Context
}

// SetupSuite runs once before all tests
func (suite *NegativeTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.T().Log("🔥 Setting up Negative Test Suite - Testing Failure Scenarios")
}

// TearDownSuite runs once after all tests
func (suite *NegativeTestSuite) TearDownSuite() {
	suite.T().Log("🧹 Tearing down Negative Test Suite")
}

// TestInvalidVaultEndpoint tests behavior with invalid Vault endpoints
func (suite *NegativeTestSuite) TestInvalidVaultEndpoint() {
	suite.T().Log("❌ Testing invalid Vault endpoint scenarios")

	testCases := []struct {
		name     string
		endpoint string
		timeout  time.Duration
	}{
		{
			name:     "non-existent host",
			endpoint: "http://non-existent-host:8200",
			timeout:  2 * time.Second,
		},
		{
			name:     "wrong port",
			endpoint: "http://localhost:9999",
			timeout:  2 * time.Second,
		},
		{
			name:     "invalid URL scheme",
			endpoint: "ftp://localhost:8200",
			timeout:  2 * time.Second,
		},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			if tc.endpoint == "ftp://localhost:8200" {
				// This should fail at client creation due to invalid URL
				_, err := client.NewClient(tc.endpoint, true, tc.timeout)
				assert.Error(t, err, "Should fail to create client with invalid URL scheme")
				return
			}

			// Create client
			vaultClient, err := client.NewClient(tc.endpoint, true, tc.timeout)
			require.NoError(t, err, "Should create client even with invalid endpoint")
			defer vaultClient.Close()

			// Health check should fail
			ctx, cancel := context.WithTimeout(suite.ctx, tc.timeout)
			defer cancel()

			_, err = vaultClient.HealthCheck(ctx)
			assert.Error(t, err, "Health check should fail for invalid endpoint: %s", tc.endpoint)

			// Seal status check should fail
			_, err = vaultClient.IsSealed(ctx)
			assert.Error(t, err, "Seal status check should fail for invalid endpoint: %s", tc.endpoint)
		})
	}
}

// TestUnsealWithInvalidKeys tests unsealing with invalid keys
func (suite *NegativeTestSuite) TestUnsealWithInvalidKeys() {
	suite.T().Log("🔑 Testing unseal with invalid keys")

	// Use a valid endpoint but non-existent for this test
	vaultClient, err := client.NewClient("http://localhost:9999", true, 2*time.Second)
	require.NoError(suite.T(), err, "Should create client")
	defer vaultClient.Close()

	ctx, cancel := context.WithTimeout(suite.ctx, 3*time.Second)
	defer cancel()

	testCases := []struct {
		name      string
		keys      []string
		threshold int
	}{
		{
			name:      "empty keys",
			keys:      []string{},
			threshold: 1,
		},
		{
			name:      "invalid base64 keys",
			keys:      []string{"not-base64!@#", "also-invalid"},
			threshold: 2,
		},
		{
			name:      "threshold exceeding keys",
			keys:      []string{base64.StdEncoding.EncodeToString([]byte("key1"))},
			threshold: 5,
		},
		{
			name: "all zero keys",
			keys: []string{
				base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 0}),
				base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 0}),
			},
			threshold: 2,
		},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			_, err := vaultClient.Unseal(ctx, tc.keys, tc.threshold)
			assert.Error(t, err, "Unseal should fail with invalid keys: %s", tc.name)
		})
	}
}

// TestTimeoutScenarios tests various timeout scenarios
func (suite *NegativeTestSuite) TestTimeoutScenarios() {
	suite.T().Log("⏰ Testing timeout scenarios")

	// Create client with very short timeout
	vaultClient, err := client.NewClient("http://httpbin.org/delay/5", true, 100*time.Millisecond)
	require.NoError(suite.T(), err, "Should create client")
	defer vaultClient.Close()

	ctx, cancel := context.WithTimeout(suite.ctx, 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = vaultClient.HealthCheck(ctx)
	duration := time.Since(start)

	// Should timeout quickly
	assert.Error(suite.T(), err, "Should timeout with slow endpoint")
	assert.True(suite.T(), duration < time.Second, "Should timeout quickly, took: %v", duration)
}

// TestConcurrentFailures tests concurrent operations that should fail
func (suite *NegativeTestSuite) TestConcurrentFailures() {
	suite.T().Log("🔀 Testing concurrent failure scenarios")

	// Create client pointing to non-existent endpoint
	vaultClient, err := client.NewClient("http://localhost:9998", true, 1*time.Second)
	require.NoError(suite.T(), err, "Should create client")
	defer vaultClient.Close()

	const numGoroutines = 5
	errorChan := make(chan error, numGoroutines)

	ctx, cancel := context.WithTimeout(suite.ctx, 3*time.Second)
	defer cancel()

	// Launch concurrent operations that should all fail
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			_, err := vaultClient.HealthCheck(ctx)
			errorChan <- err
		}(i)
	}

	// Collect all errors
	errorCount := 0
	for i := 0; i < numGoroutines; i++ {
		err := <-errorChan
		if err != nil {
			errorCount++
		}
	}

	// All operations should fail
	assert.Equal(suite.T(), numGoroutines, errorCount, "All concurrent operations should fail")
}

// TestResourceExhaustion tests behavior under resource constraints
func (suite *NegativeTestSuite) TestResourceExhaustion() {
	suite.T().Log("💾 Testing resource exhaustion scenarios")

	// Create many clients to test resource limits
	const numClients = 50
	clients := make([]*client.Client, numClients)

	// Create many clients
	for i := 0; i < numClients; i++ {
		vaultClient, err := client.NewClient("http://localhost:9997", true, 500*time.Millisecond)
		require.NoError(suite.T(), err, "Should create client %d", i)
		clients[i] = vaultClient
	}

	// Cleanup all clients
	defer func() {
		for _, c := range clients {
			if c != nil {
				c.Close()
			}
		}
	}()

	// Try operations on all clients - they should all fail but not crash
	ctx, cancel := context.WithTimeout(suite.ctx, 5*time.Second)
	defer cancel()

	errorChan := make(chan error, numClients)
	for i, vaultClient := range clients {
		go func(index int, c *client.Client) {
			_, err := c.IsSealed(ctx)
			errorChan <- err
		}(i, vaultClient)
	}

	// Collect results
	failureCount := 0
	for i := 0; i < numClients; i++ {
		err := <-errorChan
		if err != nil {
			failureCount++
		}
	}

	// Most or all should fail (no crashes)
	assert.Greater(suite.T(), failureCount, numClients/2, "Most operations should fail gracefully")
	suite.T().Logf("📊 Resource exhaustion test: %d/%d operations failed gracefully", failureCount, numClients)
}

// TestInvalidConfigurationCombinations tests invalid configuration combinations
func (suite *NegativeTestSuite) TestInvalidConfigurationCombinations() {
	suite.T().Log("⚙️ Testing invalid configuration combinations")

	testCases := []struct {
		name          string
		url           string
		tlsSkipVerify bool
		timeout       time.Duration
		shouldFail    bool
	}{
		{
			name:          "empty URL",
			url:           "",
			tlsSkipVerify: false,
			timeout:       5 * time.Second,
			shouldFail:    true,
		},
		{
			name:          "extremely short timeout",
			url:           "http://localhost:8200",
			tlsSkipVerify: false,
			timeout:       0,
			shouldFail:    true,
		},
		{
			name:          "negative timeout",
			url:           "http://localhost:8200",
			tlsSkipVerify: false,
			timeout:       -1 * time.Second,
			shouldFail:    true,
		},
		{
			name:          "invalid URL format",
			url:           "not-a-url",
			tlsSkipVerify: false,
			timeout:       5 * time.Second,
			shouldFail:    true,
		},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			_, err := client.NewClient(tc.url, tc.tlsSkipVerify, tc.timeout)

			if tc.shouldFail {
				assert.Error(t, err, "Should fail to create client with invalid config: %s", tc.name)
			} else {
				assert.NoError(t, err, "Should succeed with valid config: %s", tc.name)
			}
		})
	}
}

// TestNegativeIntegrationSuite runs the negative test suite
func TestNegativeIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping negative integration tests in short mode")
	}
	suite.Run(t, new(NegativeTestSuite))
}
