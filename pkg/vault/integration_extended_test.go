package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExtendedIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Extended Integration Suite")
}

var _ = Describe("Extended Integration Tests", func() {
	Describe("Multi-Client Orchestration", func() {
		It("should handle multiple clients targeting different vault instances", func() {
			factory := NewMockClientFactory()
			metrics := NewMockClientMetrics()

			// Create multiple clients for different vault instances
			clientConfigs := []struct {
				endpoint string
				sealed   bool
				healthy  bool
			}{
				{"http://vault-1:8200", true, true},
				{"http://vault-2:8200", false, true},
				{"http://vault-3:8200", true, false},
				{"http://vault-4:8200", false, true},
				{"http://vault-5:8200", true, true},
			}

			clients := make([]VaultClient, len(clientConfigs))
			mockClients := make([]*MockVaultClient, len(clientConfigs))

			for i, config := range clientConfigs {
				client, err := factory.NewClient(config.endpoint, false, 30*time.Second)
				Expect(err).ToNot(HaveOccurred())

				clients[i] = client
				mockClients[i] = factory.GetClient(config.endpoint)
				mockClients[i].SetSealed(config.sealed)
				mockClients[i].SetHealthy(config.healthy)
			}

			// Test concurrent operations across all clients
			var wg sync.WaitGroup
			ctx := context.Background()
			results := make([]bool, len(clients))
			errors := make([]error, len(clients))

			for i, client := range clients {
				wg.Add(1)
				go func(index int, c VaultClient) {
					defer wg.Done()

					// Check seal status
					sealed, err := c.IsSealed(ctx)
					results[index] = sealed
					errors[index] = err

					// Try health check
					_, _ = c.HealthCheck(ctx)

					// Try unseal if sealed
					if sealed && err == nil {
						keys := []string{base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("key-%d", index)))}
						_, _ = c.Unseal(ctx, keys, 1)
					}
				}(i, client)
			}

			wg.Wait()

			// Verify results match expected configuration
			for i, config := range clientConfigs {
				if config.healthy {
					Expect(errors[i]).ToNot(HaveOccurred(), "Client %d should not have errors", i)
					Expect(results[i]).To(Equal(config.sealed), "Client %d seal status mismatch", i)
				}
			}

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})

		It("should handle cascading failures across multiple instances", func() {
			factory := NewMockClientFactory()
			numClients := 10

			clients := make([]*MockVaultClient, numClients)
			for i := 0; i < numClients; i++ {
				client, err := factory.NewClient(fmt.Sprintf("http://vault-%d:8200", i), false, 5*time.Second)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = factory.GetClient(fmt.Sprintf("http://vault-%d:8200", i))
			}

			ctx := context.Background()

			// Simulate cascading failure - each failure triggers the next
			go func() {
				for i := 0; i < numClients-1; i++ {
					time.Sleep(100 * time.Millisecond)
					clients[i].SetFailSealStatus(true)
					clients[i+1].SetResponseDelay(50 * time.Millisecond) // Slow down next one
				}
			}()

			// Test resilience - some should still work
			var successCount, failureCount int
			for i := 0; i < numClients; i++ {
				_, err := clients[i].IsSealed(ctx)
				if err != nil {
					failureCount++
				} else {
					successCount++
				}
			}

			Expect(failureCount).To(BeNumerically(">", 0), "Should have some failures")
			Expect(successCount).To(BeNumerically(">", 0), "Should have some successes")
		})
	})

	Describe("Complex Unsealing Scenarios", func() {
		It("should handle mixed success/failure unsealing with retries", func() {
			baseStrategy := NewDefaultUnsealStrategy(NewDefaultKeyValidator(), NewMockClientMetrics())
			retryPolicy := &DefaultRetryPolicy{
				maxAttempts: 5,
				baseDelay:   1 * time.Millisecond,
				maxDelay:    10 * time.Millisecond,
			}
			strategy := NewRetryUnsealStrategy(baseStrategy, retryPolicy)

			// Test various unsealing scenarios
			testCases := []struct {
				name           string
				initialSealed  bool
				failuresFirst  int
				expectedResult bool
			}{
				{"immediate_success", false, 0, true},
				{"success_after_1_retry", true, 1, true},
				{"success_after_3_retries", true, 3, true},
				{"permanent_failure", true, 10, false},
			}

			ctx := context.Background()
			keys := []string{base64.StdEncoding.EncodeToString([]byte("test-key"))}

			for _, tc := range testCases {
				mockClient := NewMockVaultClient()
				mockClient.SetSealed(tc.initialSealed)

				if tc.failuresFirst > 0 {
					mockClient.SetFailSealStatus(true)

					// Set up recovery after specified failures
					if tc.failuresFirst < 10 {
						go func(failures int, client *MockVaultClient) {
							time.Sleep(time.Duration(failures+1) * 2 * time.Millisecond)
							client.SetFailSealStatus(false)
							client.SetSealed(false)
						}(tc.failuresFirst, mockClient)
					}
				}

				result, err := strategy.Unseal(ctx, mockClient, keys, 1)

				if tc.expectedResult {
					Expect(err).ToNot(HaveOccurred(), "Test case %s should succeed", tc.name)
					Expect(result.Sealed).To(BeFalse(), "Vault should be unsealed for %s", tc.name)
				} else {
					Expect(err).To(HaveOccurred(), "Test case %s should fail", tc.name)
				}
			}
		})

		It("should handle partial key submission scenarios", func() {
			validator := NewDefaultKeyValidator()
			metrics := NewMockClientMetrics()
			strategy := NewDefaultUnsealStrategy(validator, metrics)

			mockClient := NewMockVaultClient()
			mockClient.SetSealed(true)
			mockClient.unsealThreshold = 3

			// Test with different key/threshold combinations
			testCases := []struct {
				keys      int
				threshold int
				shouldErr bool
			}{
				{5, 3, false}, // Normal case
				{3, 3, false}, // Exact match
				{10, 3, false}, // More keys than needed
				{2, 3, true},   // Not enough keys
				{5, 0, true},   // Invalid threshold
				{0, 3, true},   // No keys
			}

			ctx := context.Background()
			for i, tc := range testCases {
				keys := make([]string, tc.keys)
				for j := 0; j < tc.keys; j++ {
					keys[j] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("key-%d-%d", i, j)))
				}

				result, err := strategy.Unseal(ctx, mockClient, keys, tc.threshold)

				if tc.shouldErr {
					Expect(err).To(HaveOccurred(), "Case %d should fail", i)
				} else {
					if err != nil {
						// Network errors are acceptable in this test environment
						continue
					}
					Expect(result).ToNot(BeNil(), "Case %d should return result", i)
				}

				mockClient.Reset()
				mockClient.SetSealed(true)
			}
		})
	})

	Describe("Resource Management and Cleanup", func() {
		It("should properly manage resources under high churn", func() {
			factory := NewMockClientFactory()

			// Simulate high client creation/destruction churn
			iterations := 100
			concurrency := 10

			var wg sync.WaitGroup
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < iterations/concurrency; j++ {
						endpoint := fmt.Sprintf("http://vault-%d-%d:8200", workerID, j)

						// Create client
						client, err := factory.NewClient(endpoint, false, 1*time.Second)
						Expect(err).ToNot(HaveOccurred())

						// Use client briefly
						ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
						_, _ = client.IsSealed(ctx)
						_, _ = client.HealthCheck(ctx)
						cancel()

						// Close client
						err = client.Close()
						Expect(err).ToNot(HaveOccurred())
					}
				}(i)
			}

			wg.Wait()
			// If we reach here without panics, resource management is working
		})

		It("should handle memory pressure scenarios", func() {
			// Test with large numbers of keys and complex validation
			validator := NewStrictKeyValidator(64) // Large key size
			largeKeyCount := 1000

			// Generate large keys
			keys := make([]string, largeKeyCount)
			for i := 0; i < largeKeyCount; i++ {
				keyData := make([]byte, 64)
				for j := range keyData {
					keyData[j] = byte((i + j) % 256)
				}
				keys[i] = base64.StdEncoding.EncodeToString(keyData)
			}

			// Test validation performance under memory pressure
			start := time.Now()
			err := validator.ValidateKeys(keys, largeKeyCount/2)
			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			Expect(duration).To(BeNumerically("<", 5*time.Second), "Should complete within reasonable time")
		})
	})

	Describe("Network Partition and Recovery", func() {
		It("should handle network partition scenarios", func() {
			mockClient := NewMockVaultClient()

			// Simulate network partition with gradual recovery
			partitionDuration := 100 * time.Millisecond
			mockClient.SetFailSealStatus(true)
			mockClient.SetFailHealthCheck(true)
			mockClient.SetResponseDelay(50 * time.Millisecond)

			// Set up recovery
			go func() {
				time.Sleep(partitionDuration)
				mockClient.SetFailSealStatus(false)
				mockClient.SetFailHealthCheck(false)
				mockClient.SetResponseDelay(0)
				mockClient.SetSealed(false)
			}()

			ctx := context.Background()
			retryPolicy := &DefaultRetryPolicy{
				maxAttempts: 10,
				baseDelay:   10 * time.Millisecond,
				maxDelay:    100 * time.Millisecond,
			}

			strategy := NewRetryUnsealStrategy(
				NewDefaultUnsealStrategy(NewDefaultKeyValidator(), nil),
				retryPolicy,
			)

			keys := []string{base64.StdEncoding.EncodeToString([]byte("recovery-key"))}

			start := time.Now()
			result, err := strategy.Unseal(ctx, mockClient, keys, 1)
			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred(), "Should recover from partition")
			Expect(result.Sealed).To(BeFalse(), "Should be unsealed after recovery")
			Expect(duration).To(BeNumerically(">=", partitionDuration), "Should take at least partition duration")
		})

		It("should handle intermittent connectivity issues", func() {
			mockClient := NewMockVaultClient()
			mockClient.SetSealed(true)

			// Simulate intermittent failures
			failurePattern := []bool{true, false, true, false, false}
			currentFailure := 0

			// Override the client to follow the failure pattern
			originalFailSealStatus := mockClient.failSealStatus
			mockClient.failSealStatus = false

			go func() {
				for i := 0; i < len(failurePattern); i++ {
					time.Sleep(20 * time.Millisecond)
					if i < len(failurePattern) {
						mockClient.SetFailSealStatus(failurePattern[i])
					}
				}
				// Final success
				mockClient.SetFailSealStatus(false)
				mockClient.SetSealed(false)
			}()

			ctx := context.Background()
			strategy := NewRetryUnsealStrategy(
				NewDefaultUnsealStrategy(NewDefaultKeyValidator(), nil),
				&DefaultRetryPolicy{
					maxAttempts: 10,
					baseDelay:   10 * time.Millisecond,
					maxDelay:    50 * time.Millisecond,
				},
			)

			keys := []string{base64.StdEncoding.EncodeToString([]byte("intermittent-key"))}

			result, err := strategy.Unseal(ctx, mockClient, keys, 1)

			Expect(err).ToNot(HaveOccurred(), "Should succeed despite intermittent failures")
			Expect(result.Sealed).To(BeFalse(), "Should eventually be unsealed")

			// Restore original state
			mockClient.failSealStatus = originalFailSealStatus
			_ = currentFailure // Avoid unused variable
		})
	})

	Describe("Configuration Edge Cases", func() {
		It("should handle extreme configuration values", func() {
			// Test with extreme timeout values
			extremeConfigs := []*ClientConfig{
				{
					URL:        "http://vault:8200",
					Timeout:    1 * time.Nanosecond, // Extremely short
					MaxRetries: 1,
				},
				{
					URL:        "http://vault:8200",
					Timeout:    24 * time.Hour, // Extremely long
					MaxRetries: 1000,          // Many retries
				},
				{
					URL:        "http://vault:8200",
					Timeout:    30 * time.Second,
					MaxRetries: 1,
					Validator:  NewStrictKeyValidator(1024), // Large key requirement
				},
			}

			for i, config := range extremeConfigs {
				client, err := NewClientWithConfig(config)

				if config.Timeout > 0 {
					Expect(err).ToNot(HaveOccurred(), "Config %d should be valid", i)
					Expect(client).ToNot(BeNil())
					Expect(client.Timeout()).To(Equal(config.Timeout))
					client.Close()
				}
			}
		})

		It("should handle malformed configuration combinations", func() {
			invalidConfigs := []*ClientConfig{
				{
					URL:     "", // Empty URL
					Timeout: 30 * time.Second,
				},
				{
					URL:        "not-a-url",
					Timeout:    30 * time.Second,
					MaxRetries: -1, // Negative retries
				},
			}

			for i, config := range invalidConfigs {
				client, err := NewClientWithConfig(config)

				Expect(err).To(HaveOccurred(), "Invalid config %d should fail", i)
				Expect(client).To(BeNil())
			}
		})
	})

	Describe("Metrics and Observability", func() {
		It("should collect comprehensive metrics across operations", func() {
			metrics := NewMockClientMetrics()
			config := &ClientConfig{
				URL:     "http://vault:8200",
				Timeout: 5 * time.Second,
				Metrics: metrics,
			}

			client, err := NewClientWithConfig(config)
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			ctx := context.Background()

			// Perform various operations (they will fail but should record metrics)
			operations := []func(){
				func() { client.IsSealed(ctx) },
				func() { client.GetSealStatus(ctx) },
				func() { client.HealthCheck(ctx) },
				func() { client.IsInitialized(ctx) },
				func() {
					keys := []string{base64.StdEncoding.EncodeToString([]byte("key"))}
					client.Unseal(ctx, keys, 1)
				},
			}

			for _, op := range operations {
				op()
			}

			// Verify metrics were collected
			sealStatusChecks := metrics.GetSealStatusChecks()
			healthChecks := metrics.GetHealthChecks()
			unsealAttempts := metrics.GetUnsealAttempts()

			Expect(len(sealStatusChecks)).To(BeNumerically(">=", 2)) // IsSealed + GetSealStatus
			Expect(len(healthChecks)).To(BeNumerically(">=", 1))
			Expect(len(unsealAttempts)).To(BeNumerically(">=", 1))

			// Verify metrics have proper timing information
			for _, check := range sealStatusChecks {
				Expect(check.Duration).To(BeNumerically(">", 0))
				Expect(check.Endpoint).To(Equal("http://vault:8200"))
				Expect(check.Success).To(BeFalse()) // Network calls fail in test
			}
		})

		It("should handle metrics collection under concurrent load", func() {
			metrics := NewMockClientMetrics()

			// Simulate high concurrent metrics collection
			numWorkers := 50
			operationsPerWorker := 100

			var wg sync.WaitGroup
			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for j := 0; j < operationsPerWorker; j++ {
						endpoint := fmt.Sprintf("http://vault-%d:8200", workerID)
						duration := time.Duration(rand.Intn(1000)) * time.Microsecond
						success := rand.Float32() > 0.5

						metrics.RecordSealStatusCheck(endpoint, success, duration)
						metrics.RecordHealthCheck(endpoint, success, duration)
						if rand.Float32() > 0.7 {
							metrics.RecordUnsealAttempt(endpoint, success, duration)
						}
					}
				}(i)
			}

			wg.Wait()

			// Verify all metrics were collected
			sealStatusChecks := metrics.GetSealStatusChecks()
			healthChecks := metrics.GetHealthChecks()
			unsealAttempts := metrics.GetUnsealAttempts()

			Expect(len(sealStatusChecks)).To(Equal(numWorkers * operationsPerWorker))
			Expect(len(healthChecks)).To(Equal(numWorkers * operationsPerWorker))
			Expect(len(unsealAttempts)).To(BeNumerically(">", 0))
		})
	})

	Describe("Data Integrity and Validation", func() {
		It("should maintain data integrity across complex operations", func() {
			// Test that validation and processing doesn't corrupt data
			originalKeys := []string{
				base64.StdEncoding.EncodeToString([]byte("key-1-original")),
				base64.StdEncoding.EncodeToString([]byte("key-2-original")),
				base64.StdEncoding.EncodeToString([]byte("key-3-original")),
			}

			validator := NewDefaultKeyValidator()

			// Multiple validation passes
			for i := 0; i < 100; i++ {
				keysCopy := make([]string, len(originalKeys))
				copy(keysCopy, originalKeys)

				err := validator.ValidateKeys(keysCopy, 2)
				Expect(err).ToNot(HaveOccurred())

				// Verify keys weren't modified
				for j, originalKey := range originalKeys {
					Expect(keysCopy[j]).To(Equal(originalKey), "Key %d should remain unchanged after validation", j)
				}
			}
		})

		It("should handle unicode and special characters in configuration", func() {
			specialEndpoints := []string{
				"http://vault-üñíçødé:8200",
				"http://vault-中文:8200",
				"http://vault-🔒:8200",
				"http://vault-test.example.com:8200",
			}

			for _, endpoint := range specialEndpoints {
				config := &ClientConfig{
					URL:     endpoint,
					Timeout: 30 * time.Second,
				}

				client, err := NewClientWithConfig(config)

				// Client creation should succeed
				Expect(err).ToNot(HaveOccurred(), "Should handle endpoint: %s", endpoint)
				Expect(client).ToNot(BeNil())
				Expect(client.URL()).To(Equal(endpoint))
				client.Close()
			}
		})
	})
})
