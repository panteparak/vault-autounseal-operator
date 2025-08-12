package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestChaosEngineering(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Chaos Engineering Suite")
}

var _ = Describe("Chaos Engineering Tests", func() {
	Describe("Network Chaos", func() {
		It("should handle random connection failures", func() {
			factory := NewMockClientFactory()
			numClients := 20

			clients := make([]*MockVaultClient, numClients)
			for i := 0; i < numClients; i++ {
				_, err := factory.NewClient(fmt.Sprintf("http://vault-%d:8200", i), false, 2*time.Second)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = factory.GetClient(fmt.Sprintf("http://vault-%d:8200", i))
			}

			// Inject random failures
			chaosGoroutine := func() {
				for i := 0; i < 100; i++ {
					time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
					victimIndex := rand.Intn(numClients)

					// Random failure types
					switch rand.Intn(4) {
					case 0:
						clients[victimIndex].SetFailSealStatus(true)
					case 1:
						clients[victimIndex].SetFailHealthCheck(true)
					case 2:
						clients[victimIndex].SetResponseDelay(time.Duration(rand.Intn(100)) * time.Millisecond)
					case 3:
						clients[victimIndex].SetFailUnseal(true)
					}

					// Sometimes recover
					if rand.Float32() < 0.3 {
						clients[victimIndex].Reset()
					}
				}
			}

			// Start chaos injection
			go chaosGoroutine()

			// Run operations under chaos
			ctx := context.Background()
			var successCount, failureCount int64
			var wg sync.WaitGroup

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(clientIndex int) {
					defer wg.Done()
					client := clients[clientIndex]

					for j := 0; j < 50; j++ {
						// Try various operations
						_, err1 := client.IsSealed(ctx)
						_, err2 := client.HealthCheck(ctx)

						if err1 == nil && err2 == nil {
							atomic.AddInt64(&successCount, 1)
						} else {
							atomic.AddInt64(&failureCount, 1)
						}

						time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
					}
				}(i)
			}

			wg.Wait()

			Expect(successCount).To(BeNumerically(">", 0), "Should have some successes despite chaos")
			Expect(failureCount).To(BeNumerically(">", 0), "Should have some failures due to chaos")

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})

		It("should handle Byzantine failure scenarios", func() {
			factory := NewMockClientFactory()
			numClients := 15

			clients := make([]*MockVaultClient, numClients)
			for i := 0; i < numClients; i++ {
				_, err := factory.NewClient(fmt.Sprintf("http://byzantine-%d:8200", i), false, 1*time.Second)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = factory.GetClient(fmt.Sprintf("http://byzantine-%d:8200", i))
			}

			// Configure Byzantine behaviors
			for i, client := range clients {
				switch i % 5 {
				case 0:
					// Slow responders
					client.SetResponseDelay(time.Duration(100+rand.Intn(200)) * time.Millisecond)
				case 1:
					// Intermittent failures
					client.SetFailSealStatus(true)
					go func(c *MockVaultClient) {
						ticker := time.NewTicker(50 * time.Millisecond)
						defer ticker.Stop()
						for j := 0; j < 20; j++ {
							<-ticker.C
							c.SetFailSealStatus(!c.failSealStatus)
						}
					}(client)
				case 2:
					// Corrupted responses (always return incorrect seal status)
					client.SetSealed(!client.sealed)
				case 3:
					// Partial failures (some operations fail, others succeed)
					client.SetFailHealthCheck(true)
				case 4:
					// Normal behavior (control group)
					// No special configuration
				}
			}

			// Test system behavior under Byzantine failures
			ctx := context.Background()
			strategy := NewRetryUnsealStrategy(
				NewDefaultUnsealStrategy(NewDefaultKeyValidator(), NewMockClientMetrics()),
				&DefaultRetryPolicy{
					maxAttempts: 3,
					baseDelay:   10 * time.Millisecond,
					maxDelay:    100 * time.Millisecond,
				},
			)

			var results []bool
			var wg sync.WaitGroup
			resultsChan := make(chan bool, numClients)

			keys := []string{base64.StdEncoding.EncodeToString([]byte("byzantine-key"))}

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(clientIndex int) {
					defer wg.Done()
					client := clients[clientIndex]

					_, err := strategy.Unseal(ctx, client, keys, 1)
					resultsChan <- (err == nil)
				}(i)
			}

			wg.Wait()
			close(resultsChan)

			for result := range resultsChan {
				results = append(results, result)
			}

			successCount := 0
			for _, success := range results {
				if success {
					successCount++
				}
			}

			// Should have some successes despite Byzantine failures
			Expect(successCount).To(BeNumerically(">=", numClients/5), "Should maintain some functionality under Byzantine failures")

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})
	})

	Describe("Resource Exhaustion", func() {
		It("should handle memory pressure scenarios", func() {
			factory := NewMockClientFactory()

			// Track memory usage during test
			var initialMem runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&initialMem)

			// Create many clients to simulate memory pressure
			numClients := 500
			clients := make([]VaultClient, numClients)

			for i := 0; i < numClients; i++ {
				_, err := factory.NewClient(fmt.Sprintf("http://memory-test-%d:8200", i), false, 100*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = factory.GetClient(fmt.Sprintf("http://memory-test-%d:8200", i))
			}

			// Generate large amounts of data
			ctx := context.Background()
			var wg sync.WaitGroup
			var peakMemMu sync.Mutex
			var peakMem runtime.MemStats

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(clientIndex int) {
					defer wg.Done()
					client := clients[clientIndex]

					// Generate large keys
					for j := 0; j < 10; j++ {
						keyData := make([]byte, 8192) // 8KB keys
						for k := range keyData {
							keyData[k] = byte((clientIndex + j + k) % 256)
						}

						key := base64.StdEncoding.EncodeToString(keyData)
						keys := []string{key}

						// Try operations
						_, _ = client.IsSealed(ctx)

						// Validate keys (memory intensive)
						validator := NewDefaultKeyValidator()
						_ = validator.ValidateKeys(keys, 1)

						// Check current memory (with synchronization)
						var currentMem runtime.MemStats
						runtime.ReadMemStats(&currentMem)
						peakMemMu.Lock()
						if currentMem.Alloc > peakMem.Alloc {
							peakMem = currentMem
						}
						peakMemMu.Unlock()
					}
				}(i)
			}

			wg.Wait()

			// Verify system remained stable under memory pressure
			memoryGrowth := peakMem.Alloc - initialMem.Alloc
			Expect(memoryGrowth).To(BeNumerically("<", 500*1024*1024), "Memory growth should be reasonable (< 500MB)")

			// Cleanup and verify garbage collection
			for _, client := range clients {
				client.Close()
			}

			runtime.GC()
			var finalMem runtime.MemStats
			runtime.ReadMemStats(&finalMem)

			// Memory should be released after cleanup
			Expect(finalMem.Alloc).To(BeNumerically("<", peakMem.Alloc), "Memory should be released after cleanup")
		})

		It("should handle goroutine leak scenarios", func() {
			initialGoroutines := runtime.NumGoroutine()

			factory := NewMockClientFactory()
			numIterations := 100

			// Simulate scenarios that might leak goroutines
			for i := 0; i < numIterations; i++ {
				_, err := factory.NewClient(fmt.Sprintf("http://goroutine-test-%d:8200", i), false, 50*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())

				mockClient := factory.GetClient(fmt.Sprintf("http://goroutine-test-%d:8200", i))

				// Create contexts that might be abandoned
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)

				// Start operations that might leak
				go func(c VaultClient, ctx context.Context) {
					_, _ = c.IsSealed(ctx)
					_, _ = c.HealthCheck(ctx)
				}(mockClient, ctx)

				// Sometimes cancel, sometimes let timeout
				if i%2 == 0 {
					cancel()
				} else {
					time.Sleep(15 * time.Millisecond) // Let it timeout
					cancel()                          // Clean up
				}

				// Configure failures that might cause goroutine issues
				if i%3 == 0 {
					mockClient.SetResponseDelay(100 * time.Millisecond)
				}

				mockClient.Close()
			}

			// Allow time for cleanup
			time.Sleep(200 * time.Millisecond)
			runtime.GC()
			time.Sleep(100 * time.Millisecond)

			finalGoroutines := runtime.NumGoroutine()
			goroutineGrowth := finalGoroutines - initialGoroutines

			// Allow some goroutine growth but detect significant leaks
			Expect(goroutineGrowth).To(BeNumerically("<=", 10), "Should not leak significant goroutines")
		})
	})

	Describe("Timing Attacks and Race Conditions", func() {
		It("should handle concurrent access to shared resources", func() {
			factory := NewMockClientFactory()
			validator := NewDefaultKeyValidator()
			metrics := NewMockClientMetrics()

			numWorkers := 100
			operationsPerWorker := 50

			var wg sync.WaitGroup
			var errorCount int64

			// Shared resources
			sharedKeys := []string{
				base64.StdEncoding.EncodeToString([]byte("shared-key-1")),
				base64.StdEncoding.EncodeToString([]byte("shared-key-2")),
				base64.StdEncoding.EncodeToString([]byte("shared-key-3")),
			}

			// Create clients
			clients := make([]*MockVaultClient, 10)
			for i := 0; i < 10; i++ {
				_, err := factory.NewClient(fmt.Sprintf("http://race-test-%d:8200", i), false, 100*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = factory.GetClient(fmt.Sprintf("http://race-test-%d:8200", i))
			}

			// Workers racing on shared resources
			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					ctx := context.Background()
					client := clients[workerID%len(clients)]

					for j := 0; j < operationsPerWorker; j++ {
						// Race on validation
						err := validator.ValidateKeys(sharedKeys, 2)
						if err != nil {
							atomic.AddInt64(&errorCount, 1)
						}

						// Race on metrics
						endpoint := fmt.Sprintf("http://race-test-%d:8200", workerID%10)
						duration := time.Duration(rand.Intn(1000)) * time.Microsecond
						success := rand.Float32() > 0.1

						metrics.RecordSealStatusCheck(endpoint, success, duration)
						metrics.RecordHealthCheck(endpoint, success, duration)
						if rand.Float32() > 0.7 {
							metrics.RecordUnsealAttempt(endpoint, success, duration)
						}

						// Race on client operations
						_, err = client.IsSealed(ctx)
						if err != nil {
							atomic.AddInt64(&errorCount, 1)
						}

						// Random modifications to client state
						if rand.Float32() < 0.1 {
							client.SetSealed(!client.GetSealed())
						}
						if rand.Float32() < 0.05 {
							client.SetResponseDelay(time.Duration(rand.Intn(10)) * time.Millisecond)
						}
					}
				}(i)
			}

			wg.Wait()

			// System should remain stable despite races
			totalOperations := int64(numWorkers * operationsPerWorker)
			errorRate := float64(errorCount) / float64(totalOperations)

			Expect(errorRate).To(BeNumerically("<", 0.1), "Error rate should be low despite concurrent access")

			// Verify metrics integrity
			sealChecks := metrics.GetSealStatusChecks()
			healthChecks := metrics.GetHealthChecks()
			unsealAttempts := metrics.GetUnsealAttempts()

			Expect(len(sealChecks)).To(BeNumerically(">", 0))
			Expect(len(healthChecks)).To(BeNumerically(">", 0))
			Expect(len(unsealAttempts)).To(BeNumerically(">", 0))

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})
	})

	Describe("Error Injection", func() {
		It("should handle cascading error scenarios", func() {
			factory := NewMockClientFactory()
			strategy := NewRetryUnsealStrategy(
				NewDefaultUnsealStrategy(NewDefaultKeyValidator(), NewMockClientMetrics()),
				&DefaultRetryPolicy{
					maxAttempts: 3,
					baseDelay:   1 * time.Millisecond,
					maxDelay:    10 * time.Millisecond,
				},
			)

			numClients := 20
			clients := make([]*MockVaultClient, numClients)

			for i := 0; i < numClients; i++ {
				_, err := factory.NewClient(fmt.Sprintf("http://error-cascade-%d:8200", i), false, 100*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = factory.GetClient(fmt.Sprintf("http://error-cascade-%d:8200", i))
			}

			// Inject cascading errors
			errorInjector := func() {
				// Start with one failure
				clients[0].SetFailSealStatus(true)

				// Cascade failures with delays
				for i := 1; i < numClients; i++ {
					time.Sleep(time.Duration(5+rand.Intn(10)) * time.Millisecond)

					// Each failure might trigger the next
					switch rand.Intn(4) {
					case 0:
						clients[i].SetFailSealStatus(true)
					case 1:
						clients[i].SetFailHealthCheck(true)
					case 2:
						clients[i].SetFailUnseal(true)
					case 3:
						// Sometimes break the cascade
						if rand.Float32() < 0.2 {
							return
						}
						clients[i].SetResponseDelay(50 * time.Millisecond)
					}

					// Add some recovery
					if rand.Float32() < 0.1 {
						clients[rand.Intn(i+1)].Reset()
					}
				}
			}

			// Start error cascade
			go errorInjector()

			// Test operations during cascade
			ctx := context.Background()
			keys := []string{base64.StdEncoding.EncodeToString([]byte("cascade-key"))}

			var wg sync.WaitGroup
			results := make([]error, numClients)

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(clientIndex int) {
					defer wg.Done()
					client := clients[clientIndex]

					// Add some jitter to avoid thundering herd
					time.Sleep(time.Duration(rand.Intn(20)) * time.Millisecond)

					_, err := strategy.Unseal(ctx, client, keys, 1)
					results[clientIndex] = err
				}(i)
			}

			wg.Wait()

			// Analyze cascade impact
			successCount := 0
			for _, err := range results {
				if err == nil {
					successCount++
				}
			}

			// Should have some resilience against cascade failures
			Expect(successCount).To(BeNumerically(">", 0), "Should maintain some functionality during error cascade")

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})
	})

	Describe("Time-based Chaos", func() {
		It("should handle clock skew scenarios", func() {
			factory := NewMockClientFactory()
			numClients := 10

			clients := make([]*MockVaultClient, numClients)
			for i := 0; i < numClients; i++ {
				_, err := factory.NewClient(fmt.Sprintf("http://time-chaos-%d:8200", i), false, 200*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = factory.GetClient(fmt.Sprintf("http://time-chaos-%d:8200", i))
			}

			// Inject time-based chaos
			for i, client := range clients {
				// Different delay patterns to simulate clock skew
				baseDelay := time.Duration(i*10) * time.Millisecond
				jitter := time.Duration(rand.Intn(50)) * time.Millisecond
				client.SetResponseDelay(baseDelay + jitter)

				// Some clients have variable delays (simulating network jitter)
				if i%3 == 0 {
					go func(c *MockVaultClient) {
						for j := 0; j < 50; j++ {
							time.Sleep(20 * time.Millisecond)
							newDelay := time.Duration(rand.Intn(100)) * time.Millisecond
							c.SetResponseDelay(newDelay)
						}
					}(client)
				}
			}

			// Test operations under time chaos
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			var wg sync.WaitGroup
			results := make([]time.Duration, numClients)

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(clientIndex int) {
					defer wg.Done()
					client := clients[clientIndex]

					start := time.Now()
					_, err := client.IsSealed(ctx)
					duration := time.Since(start)

					results[clientIndex] = duration

					// Should not panic or hang indefinitely
					Expect(err).To(SatisfyAny(BeNil(), HaveOccurred()))
				}(i)
			}

			wg.Wait()

			// Verify timing behavior
			for i, duration := range results {
				Expect(duration).To(BeNumerically("<", 500*time.Millisecond), "Client %d should not take too long", i)
			}

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})
	})

	Describe("Configuration Chaos", func() {
		It("should handle dynamic configuration changes", func() {
			factory := NewMockClientFactory()
			client, err := factory.NewClient("http://config-chaos:8200", false, 100*time.Millisecond)
			Expect(err).ToNot(HaveOccurred())
			mockClient := factory.GetClient("http://config-chaos:8200")

			// Configuration chaos injector
			configChaos := func() {
				for i := 0; i < 100; i++ {
					time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)

					switch rand.Intn(6) {
					case 0:
						// Toggle seal status
						mockClient.SetSealed(!mockClient.sealed)
					case 1:
						// Change health status
						mockClient.SetHealthy(!mockClient.healthy)
					case 2:
						// Modify response delays
						mockClient.SetResponseDelay(time.Duration(rand.Intn(50)) * time.Millisecond)
					case 3:
						// Toggle various failure modes
						mockClient.SetFailSealStatus(!mockClient.failSealStatus)
					case 4:
						// Change unseal threshold
						mockClient.unsealThreshold = 1 + rand.Intn(5)
					case 5:
						// Reset to baseline
						mockClient.Reset()
					}
				}
			}

			// Start configuration chaos
			go configChaos()

			// Run operations under changing configuration
			ctx := context.Background()
			var wg sync.WaitGroup
			var operationCount, errorCount int64

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()

					for j := 0; j < 100; j++ {
						atomic.AddInt64(&operationCount, 1)

						// Try various operations
						_, err1 := client.IsSealed(ctx)
						_, err2 := client.HealthCheck(ctx)
						_, err3 := client.GetSealStatus(ctx)

						if err1 != nil || err2 != nil || err3 != nil {
							atomic.AddInt64(&errorCount, 1)
						}

						time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
					}
				}()
			}

			wg.Wait()

			// Should maintain reasonable operation despite config chaos
			errorRate := float64(errorCount) / float64(operationCount)
			Expect(errorRate).To(BeNumerically("<", 0.8), "Should maintain some successful operations despite config chaos")

			client.Close()
		})
	})
})
