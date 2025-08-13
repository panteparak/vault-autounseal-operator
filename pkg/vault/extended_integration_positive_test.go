// +build integration

package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExtendedIntegrationPositive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Extended Integration Positive Test Suite")
}

var _ = Describe("Extended Integration Positive Tests", func() {
	Describe("Multi-Client Success Scenarios", func() {
		It("should successfully coordinate multiple client operations", func() {
			factory := NewMockClientFactory()
			numClients := 5

			clients := make([]VaultClient, numClients)
			mockClients := make([]*MockVaultClient, numClients)

			// Set up multiple healthy clients
			for i := 0; i < numClients; i++ {
				endpoint := fmt.Sprintf("http://vault-%d:8200", i)
				client, err := factory.NewClient(endpoint, false, 30*time.Second)
				Expect(err).ToNot(HaveOccurred())

				clients[i] = client
				mockClients[i] = factory.GetClient(endpoint)
				mockClients[i].SetHealthy(true)
				mockClients[i].SetSealed(i%2 == 0) // Alternate sealed/unsealed
			}

			ctx := context.Background()
			var wg sync.WaitGroup
			results := make([]bool, numClients)
			errors := make([]error, numClients)

			// Perform health checks concurrently
			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					health, err := clients[index].HealthCheck(ctx)
					results[index] = health != nil
					errors[index] = err
				}(i)
			}

			wg.Wait()

			// All operations should succeed
			for i := 0; i < numClients; i++ {
				Expect(errors[i]).ToNot(HaveOccurred(), "Client %d should not error", i)
				Expect(results[i]).To(BeTrue(), "Client %d should return health data", i)
			}

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})

		It("should handle successful batch unsealing operations", func() {
			validator := NewDefaultKeyValidator()
			metrics := NewMockClientMetrics()
			strategy := NewDefaultUnsealStrategy(validator, metrics)

			numOperations := 10
			keys := []string{base64.StdEncoding.EncodeToString([]byte("batch-key"))}

			var wg sync.WaitGroup
			successes := make([]bool, numOperations)

			for i := 0; i < numOperations; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					// Each operation gets its own client
					client := NewMockVaultClient()
					client.SetSealed(true)
					client.SetHealthy(true)

					_, err := strategy.Unseal(context.Background(), client, keys, 1)
					successes[index] = err == nil
				}(i)
			}

			wg.Wait()

			// All unsealing operations should succeed
			successCount := 0
			for _, success := range successes {
				if success {
					successCount++
				}
			}

			Expect(successCount).To(Equal(numOperations), "All batch operations should succeed")
		})
	})

	Describe("Graceful Degradation Scenarios", func() {
		It("should gracefully handle mixed client states", func() {
			factory := NewMockClientFactory()

			// Create clients with various states
			scenarios := []struct {
				endpoint string
				sealed   bool
				healthy  bool
				delay    time.Duration
			}{
				{"http://fast-healthy:8200", false, true, 0},
				{"http://slow-healthy:8200", true, true, 10 * time.Millisecond},
				{"http://fast-sealed:8200", true, true, 0},
				{"http://slow-sealed:8200", true, true, 20 * time.Millisecond},
			}

			ctx := context.Background()
			results := make(map[string]bool)

			for _, scenario := range scenarios {
				client, err := factory.NewClient(scenario.endpoint, false, 100*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())

				mockClient := factory.GetClient(scenario.endpoint)
				mockClient.SetSealed(scenario.sealed)
				mockClient.SetHealthy(scenario.healthy)
				mockClient.SetResponseDelay(scenario.delay)

				// Test that each client responds according to its configuration
				sealed, err := client.IsSealed(ctx)
				Expect(err).ToNot(HaveOccurred())
				results[scenario.endpoint] = sealed

				client.Close()
			}

			// Verify each client behaved as expected
			Expect(results["http://fast-healthy:8200"]).To(BeFalse())
			Expect(results["http://slow-healthy:8200"]).To(BeTrue())
			Expect(results["http://fast-sealed:8200"]).To(BeTrue())
			Expect(results["http://slow-sealed:8200"]).To(BeTrue())
		})

		It("should maintain performance under varied load", func() {
			factory := NewMockClientFactory()

			// Create client with metrics
			client, err := factory.NewClient("http://perf-test:8200", false, 100*time.Millisecond)
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			mockClient := factory.GetClient("http://perf-test:8200")
			mockClient.SetHealthy(true)

			ctx := context.Background()
			numOperations := 100
			start := time.Now()

			// Perform operations under load
			var wg sync.WaitGroup
			for i := 0; i < numOperations; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _ = client.IsSealed(ctx)
					_, _ = client.HealthCheck(ctx)
				}()
			}

			wg.Wait()
			duration := time.Since(start)

			// Performance should be reasonable
			avgLatency := duration / time.Duration(numOperations*2) // 2 operations per iteration
			Expect(avgLatency).To(BeNumerically("<", 5*time.Millisecond), "Average latency should be low")
		})
	})

	Describe("Advanced Configuration Scenarios", func() {
		It("should handle complex client configurations successfully", func() {
			complexConfigs := []*ClientConfig{
				{
					URL:           "https://prod-vault.example.com:8200",
					Timeout:       45 * time.Second,
					MaxRetries:    5,
					RetryDelay:    2 * time.Second,
					TLSSkipVerify: false,
					Validator:     NewDefaultKeyValidator(),
					Metrics:       NewMockClientMetrics(),
				},
				{
					URL:           "http://dev-vault:8200",
					Timeout:       15 * time.Second,
					MaxRetries:    3,
					RetryDelay:    500 * time.Millisecond,
					TLSSkipVerify: true,
					Validator:     NewStrictKeyValidator(32),
					Metrics:       NewMockClientMetrics(),
				},
			}

			for i, config := range complexConfigs {
				client, err := NewClientWithConfig(config)
				Expect(err).ToNot(HaveOccurred(), "Complex config %d should be valid", i)
				Expect(client).ToNot(BeNil())

				// Verify configuration was applied
				Expect(client.URL()).To(Equal(config.URL))
				Expect(client.Timeout()).To(Equal(config.Timeout))

				client.Close()
			}
		})

		It("should handle custom strategies and validators", func() {
			// Create custom validator with specific requirements
			strictValidator := NewStrictKeyValidator(64)
			strictValidator.SetAllowedPrefixes([]string{"prod_", "staging_"})
			strictValidator.SetForbiddenStrings([]string{"test", "debug"})

			// Create custom metrics
			customMetrics := NewMockClientMetrics()

			// Create strategies with custom components
			baseStrategy := NewDefaultUnsealStrategy(strictValidator, customMetrics)
			retryPolicy := &DefaultRetryPolicy{
				maxAttempts: 5,
				baseDelay:   100 * time.Millisecond,
				maxDelay:    5 * time.Second,
			}
			retryStrategy := NewRetryUnsealStrategy(baseStrategy, retryPolicy)

			// Test with valid keys according to strict validator
			validKey := make([]byte, 64)
			copy(validKey, "prod_secure_key_data_with_sufficient_length_and_entropy")
			validKeyB64 := base64.StdEncoding.EncodeToString(validKey)

			mockClient := NewMockVaultClient()
			mockClient.SetSealed(true)

			result, err := retryStrategy.Unseal(context.Background(), mockClient, []string{validKeyB64}, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			// Verify metrics were recorded
			unsealAttempts := customMetrics.GetUnsealAttempts()
			Expect(len(unsealAttempts)).To(BeNumerically(">=", 1))
		})
	})

	Describe("State Management Scenarios", func() {
		It("should maintain consistent state across operations", func() {
			mockClient := NewMockVaultClient()
			ctx := context.Background()

			// Initial state - sealed
			mockClient.SetSealed(true)
			sealed, err := mockClient.IsSealed(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(sealed).To(BeTrue())

			// Simulate partial unsealing progress
			mockClient.unsealProgress = 2
			mockClient.unsealThreshold = 3
			status, err := mockClient.GetSealStatus(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(status.Progress).To(Equal(2))
			Expect(status.T).To(Equal(3))

			// Complete unsealing
			mockClient.SetSealed(false)
			sealed, err = mockClient.IsSealed(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(sealed).To(BeFalse())

			// Verify final state
			status, err = mockClient.GetSealStatus(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(status.Sealed).To(BeFalse())
		})

		It("should handle state transitions gracefully", func() {
			factory := NewMockClientFactory()
			client, err := factory.NewClient("http://state-test:8200", false, 30*time.Second)
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			mockClient := factory.GetClient("http://state-test:8200")
			ctx := context.Background()

			// Test sequence: healthy -> unhealthy -> healthy
			states := []struct {
				healthy       bool
				sealed        bool
				expectHealthy bool
				expectSealed  bool
			}{
				{true, true, true, true},
				{false, true, false, true},  // Unhealthy phase
				{true, false, true, false}, // Recovery
			}

			for i, state := range states {
				mockClient.SetHealthy(state.healthy)
				mockClient.SetSealed(state.sealed)

				if state.healthy {
					health, err := client.HealthCheck(ctx)
					Expect(err).ToNot(HaveOccurred(), "Health check %d should succeed", i)
					Expect(health.Sealed).To(Equal(state.expectSealed))

					sealed, err := client.IsSealed(ctx)
					Expect(err).ToNot(HaveOccurred(), "Seal check %d should succeed", i)
					Expect(sealed).To(Equal(state.expectSealed))
				} else {
					_, err := client.HealthCheck(ctx)
					Expect(err).To(HaveOccurred(), "Health check %d should fail when unhealthy", i)
				}
			}
		})
	})

	Describe("Concurrent Operation Success", func() {
		It("should handle high concurrency successfully", func() {
			factory := NewMockClientFactory()
			numWorkers := 50
			operationsPerWorker := 20

			client, err := factory.NewClient("http://concurrency-test:8200", false, 100*time.Millisecond)
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			mockClient := factory.GetClient("http://concurrency-test:8200")
			mockClient.SetHealthy(true)
			mockClient.SetSealed(false)

			ctx := context.Background()
			var wg sync.WaitGroup
			var totalOperations, successfulOperations int64

			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()

					for j := 0; j < operationsPerWorker; j++ {
						totalOperations++

						// Mix of operations
						switch j % 3 {
						case 0:
							_, err := client.IsSealed(ctx)
							if err == nil {
								successfulOperations++
							}
						case 1:
							_, err := client.HealthCheck(ctx)
							if err == nil {
								successfulOperations++
							}
						case 2:
							_, err := client.IsInitialized(ctx)
							if err == nil {
								successfulOperations++
							}
						}
					}
				}()
			}

			wg.Wait()

			// High success rate expected under normal conditions
			successRate := float64(successfulOperations) / float64(totalOperations)
			Expect(successRate).To(BeNumerically(">", 0.95), "Success rate should be high under normal conditions")
			Expect(totalOperations).To(Equal(int64(numWorkers * operationsPerWorker)))
		})

		It("should coordinate multiple unsealing operations successfully", func() {
			validator := NewDefaultKeyValidator()
			metrics := NewMockClientMetrics()

			numClients := 10
			keys := []string{base64.StdEncoding.EncodeToString([]byte("coordination-key"))}

			var wg sync.WaitGroup
			results := make([]error, numClients)

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					// Each goroutine gets its own client and strategy
					mockClient := NewMockVaultClient()
					mockClient.SetSealed(true)
					mockClient.SetHealthy(true)

					strategy := NewDefaultUnsealStrategy(validator, metrics)
					_, err := strategy.Unseal(context.Background(), mockClient, keys, 1)
					results[index] = err
				}(i)
			}

			wg.Wait()

			// All operations should succeed
			successCount := 0
			for _, err := range results {
				if err == nil {
					successCount++
				}
			}

			Expect(successCount).To(Equal(numClients), "All concurrent unsealing operations should succeed")

			// Verify metrics were collected from all operations
			unsealAttempts := metrics.GetUnsealAttempts()
			Expect(len(unsealAttempts)).To(Equal(numClients))
		})
	})

	Describe("Resource Management Success", func() {
		It("should manage resources efficiently under sustained load", func() {
			factory := NewMockClientFactory()

			// Create and destroy clients rapidly to test resource management
			numIterations := 100
			clientsPerIteration := 5

			for i := 0; i < numIterations; i++ {
				clients := make([]VaultClient, clientsPerIteration)

				// Create clients
				for j := 0; j < clientsPerIteration; j++ {
					endpoint := fmt.Sprintf("http://resource-test-%d-%d:8200", i, j)
					client, err := factory.NewClient(endpoint, false, 10*time.Millisecond)
					Expect(err).ToNot(HaveOccurred())
					clients[j] = client
				}

				// Use clients briefly
				ctx := context.Background()
				for _, client := range clients {
					_, _ = client.IsSealed(ctx) // Ignore errors, just test resource usage
				}

				// Clean up
				for _, client := range clients {
					err := client.Close()
					Expect(err).ToNot(HaveOccurred())
				}
			}

			// If we reach here without panics or resource exhaustion, test passes
		})

		It("should handle graceful client lifecycle", func() {
			factory := NewMockClientFactory()

			client, err := factory.NewClient("http://lifecycle-test:8200", false, 30*time.Second)
			Expect(err).ToNot(HaveOccurred())

			mockClient := factory.GetClient("http://lifecycle-test:8200")
			mockClient.SetHealthy(true)

			// Client should be usable
			ctx := context.Background()
			_, err = client.IsSealed(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Client should report not closed initially
			Expect(client.IsClosed()).To(BeFalse())

			// Close client
			err = client.Close()
			Expect(err).ToNot(HaveOccurred())

			// Client should report closed
			Expect(client.IsClosed()).To(BeTrue())

			// Multiple close calls should be safe
			err = client.Close()
			Expect(err).ToNot(HaveOccurred())

			// Operations on closed client should fail gracefully
			_, err = client.IsSealed(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("closed"))
		})
	})
})
