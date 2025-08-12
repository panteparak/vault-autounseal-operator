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

func TestLoadTesting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Load Testing Suite")
}

var _ = Describe("Load and Stress Tests", func() {
	Describe("High Volume Operations", func() {
		It("should handle high volume seal status checks", func() {
			factory := NewMockClientFactory()
			numClients := 100
			operationsPerClient := 1000

			clients := make([]*MockVaultClient, numClients)
			for i := 0; i < numClients; i++ {
				_, err := factory.NewClient(fmt.Sprintf("http://load-test-%d:8200", i), false, 50*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = factory.GetClient(fmt.Sprintf("http://load-test-%d:8200", i))

				// Vary client configurations for realism
				clients[i].SetSealed(rand.Float32() < 0.5)
				clients[i].SetHealthy(rand.Float32() < 0.9) // 90% healthy
				if rand.Float32() < 0.1 {
					clients[i].SetResponseDelay(time.Duration(rand.Intn(20)) * time.Millisecond)
				}
			}

			// Track performance metrics
			var totalOperations, successfulOperations int64
			var totalDuration int64
			startTime := time.Now()

			var wg sync.WaitGroup
			ctx := context.Background()

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(clientIndex int) {
					defer wg.Done()
					client := clients[clientIndex]

					for j := 0; j < operationsPerClient; j++ {
						opStart := time.Now()
						atomic.AddInt64(&totalOperations, 1)

						_, err := client.IsSealed(ctx)

						opDuration := time.Since(opStart)
						atomic.AddInt64(&totalDuration, int64(opDuration))

						if err == nil {
							atomic.AddInt64(&successfulOperations, 1)
						}

						// Small delay to avoid overwhelming
						if j%100 == 0 {
							time.Sleep(time.Millisecond)
						}
					}
				}(i)
			}

			wg.Wait()
			totalTestDuration := time.Since(startTime)

			// Performance assertions
			avgLatency := time.Duration(totalDuration / totalOperations)
			throughput := float64(totalOperations) / totalTestDuration.Seconds()
			successRate := float64(successfulOperations) / float64(totalOperations)

			Expect(avgLatency).To(BeNumerically("<", 10*time.Millisecond), "Average latency should be reasonable")
			Expect(throughput).To(BeNumerically(">", 1000), "Should achieve good throughput (>1000 ops/sec)")
			Expect(successRate).To(BeNumerically(">", 0.8), "Success rate should be high (>80%)")

			fmt.Printf("Load Test Results:\n")
			fmt.Printf("  Total Operations: %d\n", totalOperations)
			fmt.Printf("  Successful Operations: %d\n", successfulOperations)
			fmt.Printf("  Success Rate: %.2f%%\n", successRate*100)
			fmt.Printf("  Average Latency: %v\n", avgLatency)
			fmt.Printf("  Throughput: %.2f ops/sec\n", throughput)
			fmt.Printf("  Total Duration: %v\n", totalTestDuration)

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})

		It("should handle high volume unseal operations", func() {
			strategy := NewDefaultUnsealStrategy(NewDefaultKeyValidator(), NewMockClientMetrics())
			_ = NewMockClientFactory()

			numClients := 50
			operationsPerClient := 200

			// Pre-generate keys to avoid key generation overhead in test
			keys := make([]string, 10)
			for i := 0; i < 10; i++ {
				keyData := make([]byte, 32)
				for j := range keyData {
					keyData[j] = byte(i*31 + j)
				}
				keys[i] = base64.StdEncoding.EncodeToString(keyData)
			}

			var totalOperations, successfulOperations int64
			var wg sync.WaitGroup
			ctx := context.Background()

			startTime := time.Now()

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(clientIndex int) {
					defer wg.Done()

					// Create client for this worker
					client := NewMockVaultClient()
					client.SetSealed(true)

					for j := 0; j < operationsPerClient; j++ {
						atomic.AddInt64(&totalOperations, 1)

						// Use subset of keys
						keySubset := keys[:3]
						_, err := strategy.Unseal(ctx, client, keySubset, 3)

						if err == nil {
							atomic.AddInt64(&successfulOperations, 1)
						}

						// Reset client state for next operation
						client.Reset()
						client.SetSealed(true)

						// Add some variability
						if rand.Float32() < 0.05 {
							time.Sleep(time.Millisecond)
						}
					}
				}(i)
			}

			wg.Wait()
			totalDuration := time.Since(startTime)

			throughput := float64(totalOperations) / totalDuration.Seconds()
			successRate := float64(successfulOperations) / float64(totalOperations)

			Expect(throughput).To(BeNumerically(">", 100), "Should achieve reasonable unseal throughput")
			Expect(successRate).To(BeNumerically(">", 0.95), "Unseal success rate should be high")

			fmt.Printf("Unseal Load Test Results:\n")
			fmt.Printf("  Throughput: %.2f unseals/sec\n", throughput)
			fmt.Printf("  Success Rate: %.2f%%\n", successRate*100)
		})
	})

	Describe("Memory Stress Tests", func() {
		It("should handle large key sets efficiently", func() {
			validator := NewDefaultKeyValidator()

			// Test with increasingly large key sets
			keySizes := []int{10, 100, 500, 1000, 2000}

			for _, keyCount := range keySizes {
				var memBefore, memAfter runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&memBefore)

				startTime := time.Now()

				// Generate large key set
				keys := make([]string, keyCount)
				for i := 0; i < keyCount; i++ {
					keyData := make([]byte, 64) // 64-byte keys
					for j := range keyData {
						keyData[j] = byte((i*17 + j*23) % 256)
					}
					keys[i] = base64.StdEncoding.EncodeToString(keyData)
				}

				// Validate keys multiple times
				for iteration := 0; iteration < 10; iteration++ {
					err := validator.ValidateKeys(keys, keyCount/2)
					Expect(err).ToNot(HaveOccurred())
				}

				duration := time.Since(startTime)
				runtime.GC()
				runtime.ReadMemStats(&memAfter)

				memoryGrowth := memAfter.Alloc - memBefore.Alloc

				// Performance expectations
				Expect(duration).To(BeNumerically("<", 5*time.Second),
					"Validation of %d keys should complete quickly", keyCount)
				Expect(memoryGrowth).To(BeNumerically("<", int64(keyCount)*1024),
					"Memory growth should be proportional to key count")

				fmt.Printf("Key Set Size %d: Duration=%v, Memory Growth=%dKB\n",
					keyCount, duration, memoryGrowth/1024)
			}
		})

		It("should handle memory pressure during concurrent operations", func() {
			factory := NewMockClientFactory()
			numWorkers := 20
			memoryPressureSize := 10 * 1024 * 1024 // 10MB per worker

			var wg sync.WaitGroup
			var totalAllocations int64

			// Track memory usage
			var peakMemory int64
			memoryMonitor := func() {
				ticker := time.NewTicker(100 * time.Millisecond)
				defer ticker.Stop()

				for i := 0; i < 50; i++ { // Monitor for 5 seconds
					<-ticker.C
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					current := int64(m.Alloc)
					if current > atomic.LoadInt64(&peakMemory) {
						atomic.StoreInt64(&peakMemory, current)
					}
				}
			}
			go memoryMonitor()

			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					// Create memory pressure
					largeData := make([][]byte, 100)
					for j := range largeData {
						largeData[j] = make([]byte, memoryPressureSize/100)
						atomic.AddInt64(&totalAllocations, int64(len(largeData[j])))

						// Fill with pattern
						for k := range largeData[j] {
							largeData[j][k] = byte((workerID + j + k) % 256)
						}
					}

					// Perform operations under memory pressure
					_, err := factory.NewClient(fmt.Sprintf("http://memory-pressure-%d:8200", workerID), false, 100*time.Millisecond)
					Expect(err).ToNot(HaveOccurred())

					mockClient := factory.GetClient(fmt.Sprintf("http://memory-pressure-%d:8200", workerID))
					ctx := context.Background()
					for j := 0; j < 50; j++ {
						_, err := mockClient.IsSealed(ctx)
						if err != nil {
							// Acceptable under memory pressure
						}

						// Process some of the large data
						if j%10 == 0 && len(largeData) > 0 {
							_ = base64.StdEncoding.EncodeToString(largeData[j%len(largeData)])
						}
					}

					mockClient.Close()

					// Clear memory
					largeData = nil
					runtime.GC()
				}(i)
			}

			wg.Wait()

			finalPeak := atomic.LoadInt64(&peakMemory)
			Expect(finalPeak).To(BeNumerically("<", 500*1024*1024), "Peak memory should be reasonable (< 500MB)")

			fmt.Printf("Memory Pressure Test:\n")
			fmt.Printf("  Peak Memory Usage: %d MB\n", finalPeak/(1024*1024))
			fmt.Printf("  Total Allocations: %d MB\n", totalAllocations/(1024*1024))
		})
	})

	Describe("Concurrency Stress Tests", func() {
		It("should handle extreme concurrency without deadlocks", func() {
			factory := NewMockClientFactory()
			metrics := NewMockClientMetrics()

			numWorkers := 200
			operationsPerWorker := 100

			// Shared resources to stress concurrency
			sharedClient, err := factory.NewClient("http://shared-client:8200", false, 50*time.Millisecond)
			Expect(err).ToNot(HaveOccurred())

			validator := NewDefaultKeyValidator()
			strategy := NewDefaultUnsealStrategy(validator, metrics)

			var wg sync.WaitGroup
			var totalOperations, errors int64

			// Use channels to create additional synchronization points
			operationChan := make(chan int, numWorkers*operationsPerWorker)
			resultChan := make(chan error, numWorkers*operationsPerWorker)

			// Fill operation channel
			for i := 0; i < numWorkers*operationsPerWorker; i++ {
				operationChan <- i
			}
			close(operationChan)

			startTime := time.Now()

			// Workers competing for shared resources
			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					ctx := context.Background()
					mockClient := NewMockVaultClient()
					keys := []string{base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("worker-%d-key", workerID)))}

					for opID := range operationChan {
						atomic.AddInt64(&totalOperations, 1)

						var err error

						// Mix of operations to stress different paths
						switch opID % 7 {
						case 0:
							_, err = sharedClient.IsSealed(ctx)
						case 1:
							_, err = sharedClient.HealthCheck(ctx)
						case 2:
							err = validator.ValidateKeys(keys, 1)
						case 3:
							_, err = strategy.Unseal(ctx, mockClient, keys, 1)
						case 4:
							metrics.RecordSealStatusCheck("test", true, time.Microsecond)
						case 5:
							_, err = mockClient.GetSealStatus(ctx)
						case 6:
							// Create and destroy clients
							tempClient, tempErr := factory.NewClient(fmt.Sprintf("http://temp-%d-%d:8200", workerID, opID), false, 10*time.Millisecond)
							if tempErr == nil {
								tempClient.Close()
							}
							err = tempErr
						}

						resultChan <- err

						// Add small delays to increase chance of races
						if opID%50 == 0 {
							runtime.Gosched()
						}
					}
				}(i)
			}

			// Result collector
			go func() {
				for i := 0; i < int(numWorkers*operationsPerWorker); i++ {
					err := <-resultChan
					if err != nil {
						atomic.AddInt64(&errors, 1)
					}
				}
				close(resultChan)
			}()

			wg.Wait()
			totalDuration := time.Since(startTime)

			// Drain result channel
			for range resultChan {
				// Already counted
			}

			errorRate := float64(errors) / float64(totalOperations)
			throughput := float64(totalOperations) / totalDuration.Seconds()

			// Should handle extreme concurrency without deadlocks
			Expect(totalDuration).To(BeNumerically("<", 30*time.Second), "Should complete without deadlocks")
			Expect(errorRate).To(BeNumerically("<", 0.2), "Error rate should be acceptable under extreme concurrency")
			Expect(throughput).To(BeNumerically(">", 100), "Should maintain reasonable throughput")

			fmt.Printf("Concurrency Stress Test:\n")
			fmt.Printf("  Workers: %d\n", numWorkers)
			fmt.Printf("  Total Operations: %d\n", totalOperations)
			fmt.Printf("  Errors: %d (%.2f%%)\n", errors, errorRate*100)
			fmt.Printf("  Throughput: %.2f ops/sec\n", throughput)
			fmt.Printf("  Duration: %v\n", totalDuration)

			sharedClient.Close()
		})

		It("should handle resource contention scenarios", func() {
			// Test with limited resources and many consumers
			factory := NewMockClientFactory()

			numClients := 5  // Limited clients
			numWorkers := 50 // Many workers competing

			clients := make([]VaultClient, numClients)
			for i := 0; i < numClients; i++ {
				_, err := factory.NewClient(fmt.Sprintf("http://contention-%d:8200", i), false, 100*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = factory.GetClient(fmt.Sprintf("http://contention-%d:8200", i))
			}

			var wg sync.WaitGroup
			var operationCount, waitTime int64

			// Workers competing for limited client resources
			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					ctx := context.Background()

					for j := 0; j < 20; j++ {
						waitStart := time.Now()

						// Random client selection (contention point)
						clientIndex := rand.Intn(numClients)
						client := clients[clientIndex]

						atomic.AddInt64(&waitTime, int64(time.Since(waitStart)))

						// Perform operation
						_, err := client.IsSealed(ctx)
						atomic.AddInt64(&operationCount, 1)

						if err != nil {
							// Expected under contention
						}

						// Hold resource briefly
						time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
					}
				}(i)
			}

			wg.Wait()

			avgWaitTime := time.Duration(waitTime / operationCount)
			Expect(avgWaitTime).To(BeNumerically("<", 10*time.Millisecond), "Wait time should be reasonable despite contention")
			Expect(operationCount).To(Equal(int64(numWorkers*20)), "All operations should complete")

			fmt.Printf("Resource Contention Test:\n")
			fmt.Printf("  Clients: %d, Workers: %d\n", numClients, numWorkers)
			fmt.Printf("  Operations: %d\n", operationCount)
			fmt.Printf("  Avg Wait Time: %v\n", avgWaitTime)

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})
	})

	Describe("Network Load Simulation", func() {
		It("should handle varying network conditions", func() {
			factory := NewMockClientFactory()
			numClients := 30

			clients := make([]*MockVaultClient, numClients)
			for i := 0; i < numClients; i++ {
				_, err := factory.NewClient(fmt.Sprintf("http://network-sim-%d:8200", i), false, 200*time.Millisecond)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = factory.GetClient(fmt.Sprintf("http://network-sim-%d:8200", i))
			}

			// Simulate various network conditions
			networkConditions := []struct {
				name    string
				delay   time.Duration
				failure float32
			}{
				{"fast", 1 * time.Millisecond, 0.01},
				{"normal", 10 * time.Millisecond, 0.05},
				{"slow", 50 * time.Millisecond, 0.1},
				{"poor", 100 * time.Millisecond, 0.2},
				{"terrible", 200 * time.Millisecond, 0.3},
			}

			// Apply network conditions
			for i, client := range clients {
				condition := networkConditions[i%len(networkConditions)]
				client.SetResponseDelay(condition.delay)

				if rand.Float32() < condition.failure {
					client.SetFailSealStatus(true)
				}
			}

			// Network condition changer
			go func() {
				ticker := time.NewTicker(50 * time.Millisecond)
				defer ticker.Stop()

				for i := 0; i < 40; i++ { // Change for 2 seconds
					<-ticker.C

					// Randomly change some client conditions
					clientIndex := rand.Intn(numClients)
					conditionIndex := rand.Intn(len(networkConditions))
					condition := networkConditions[conditionIndex]

					clients[clientIndex].SetResponseDelay(condition.delay)
					clients[clientIndex].SetFailSealStatus(rand.Float32() < condition.failure)
				}
			}()

			// Test operations under varying network conditions
			var wg sync.WaitGroup
			var totalOperations, successfulOperations int64
			var totalLatency int64

			ctx := context.Background()

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(clientIndex int) {
					defer wg.Done()
					client := clients[clientIndex]

					for j := 0; j < 50; j++ {
						start := time.Now()
						atomic.AddInt64(&totalOperations, 1)

						_, err := client.IsSealed(ctx)

						latency := time.Since(start)
						atomic.AddInt64(&totalLatency, int64(latency))

						if err == nil {
							atomic.AddInt64(&successfulOperations, 1)
						}
					}
				}(i)
			}

			wg.Wait()

			avgLatency := time.Duration(totalLatency / totalOperations)
			successRate := float64(successfulOperations) / float64(totalOperations)

			// Should adapt to varying network conditions
			Expect(successRate).To(BeNumerically(">", 0.6), "Should maintain reasonable success rate under varying network conditions")
			Expect(avgLatency).To(BeNumerically("<", 300*time.Millisecond), "Average latency should be acceptable")

			fmt.Printf("Network Load Simulation:\n")
			fmt.Printf("  Success Rate: %.2f%%\n", successRate*100)
			fmt.Printf("  Average Latency: %v\n", avgLatency)

			// Cleanup
			for _, client := range clients {
				client.Close()
			}
		})
	})

	Describe("Long-Running Stress Tests", func() {
		It("should maintain stability over extended periods", func() {
			if testing.Short() {
				Skip("Skipping long-running test in short mode")
			}

			factory := NewMockClientFactory()
			client, err := factory.NewClient("http://stability-test:8200", false, 100*time.Millisecond)
			Expect(err).ToNot(HaveOccurred())

			// Track metrics over time
			var operationCount, errorCount int64
			var minLatency, maxLatency int64 = 999999999, 0

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Stability test worker
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()

				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						start := time.Now()
						atomic.AddInt64(&operationCount, 1)

						_, err := client.IsSealed(ctx)

						latency := int64(time.Since(start))

						// Update latency stats
						for {
							current := atomic.LoadInt64(&minLatency)
							if latency >= current || atomic.CompareAndSwapInt64(&minLatency, current, latency) {
								break
							}
						}
						for {
							current := atomic.LoadInt64(&maxLatency)
							if latency <= current || atomic.CompareAndSwapInt64(&maxLatency, current, latency) {
								break
							}
						}

						if err != nil {
							atomic.AddInt64(&errorCount, 1)
						}
					}
				}
			}()

			wg.Wait()

			errorRate := float64(errorCount) / float64(operationCount)
			minLat := time.Duration(atomic.LoadInt64(&minLatency))
			maxLat := time.Duration(atomic.LoadInt64(&maxLatency))

			Expect(operationCount).To(BeNumerically(">", 1000), "Should perform many operations")
			Expect(errorRate).To(BeNumerically("<", 0.1), "Error rate should remain low over time")
			Expect(maxLat).To(BeNumerically("<", 500*time.Millisecond), "Max latency should be bounded")

			fmt.Printf("Stability Test (30s):\n")
			fmt.Printf("  Operations: %d\n", operationCount)
			fmt.Printf("  Error Rate: %.2f%%\n", errorRate*100)
			fmt.Printf("  Latency Range: %v - %v\n", minLat, maxLat)

			client.Close()
		})
	})
})
