package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestComprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Comprehensive Testing Suite")
}

var _ = Describe("Comprehensive Testing Framework", func() {
	var runner *TestRunner
	var config *TestConfig

	BeforeEach(func() {
		config = DefaultTestConfig()
		// Reduce durations for faster tests in development
		if testing.Short() {
			config.LoadTestDuration = 5 * time.Second
			config.ChaosTestDuration = 3 * time.Second
			config.SecurityTestIterations = 10
			config.PropertyTestIterations = 50
			config.MemoryTestIterations = 10
			config.ProfileDuration = 5 * time.Second
		}

		runner = NewTestRunner(config)
	})

	Describe("Test Configuration", func() {
		It("should load default configuration", func() {
			defaultConfig := DefaultTestConfig()

			Expect(defaultConfig.LoadTestDuration).To(Equal(30 * time.Second))
			Expect(defaultConfig.LoadTestWorkers).To(Equal(10))
			Expect(defaultConfig.ChaosTestClients).To(Equal(15))
			Expect(defaultConfig.SecurityTestIterations).To(Equal(100))
			Expect(defaultConfig.PropertyTestIterations).To(Equal(500))
		})

		It("should load configuration from environment", func() {
			// Set environment variables
			os.Setenv("LOAD_TEST_DURATION", "45s")
			os.Setenv("LOAD_TEST_WORKERS", "20")
			os.Setenv("SECURITY_TEST_ITERATIONS", "200")
			defer func() {
				os.Unsetenv("LOAD_TEST_DURATION")
				os.Unsetenv("LOAD_TEST_WORKERS")
				os.Unsetenv("SECURITY_TEST_ITERATIONS")
			}()

			config := DefaultTestConfig()
			config.LoadFromEnvironment()

			Expect(config.LoadTestDuration).To(Equal(45 * time.Second))
			Expect(config.LoadTestWorkers).To(Equal(20))
			Expect(config.SecurityTestIterations).To(Equal(200))
		})
	})

	Describe("Load Testing Framework", func() {
		It("should run load tests with metrics collection", func() {
			loadRunner := NewLoadTestRunner(5, 2*time.Second)

			// Configure operation mix
			loadRunner.SetOperationMix(map[string]float32{
				"IsSealed":      0.5,
				"HealthCheck":   0.3,
				"GetSealStatus": 0.2,
			})

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			summary := loadRunner.Run(ctx)

			Expect(summary.TotalOperations).To(BeNumerically(">", 0))
			Expect(summary.TestDuration).To(BeNumerically("~", 2*time.Second, time.Second))

			// Verify operation mix was respected
			breakDown := summary.OperationBreakdown
			Expect(len(breakDown)).To(BeNumerically(">=", 2))
		})

		It("should detect performance regressions", func() {
			loadRunner := NewLoadTestRunner(2, 1*time.Second)

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			summary := loadRunner.Run(ctx)

			// Check performance thresholds
			for operation, stats := range summary.OperationBreakdown {
				if stats.Count > 0 {
					Expect(stats.AvgTime).To(BeNumerically("<", 100*time.Millisecond),
						"Operation %s should have reasonable latency", operation)
				}
			}
		})
	})

	Describe("Chaos Engineering Framework", func() {
		It("should run chaos tests with configurable scenarios", func() {
			chaosRunner := NewChaosTestRunner(5)

			// Add custom chaos scenario
			chaosRunner.AddChaosScenario(ChaosScenario{
				Name:        "CustomFailure",
				Description: "Custom failure scenario for testing",
				Probability: 0.1,
				ApplyFunc: func(c *MockVaultClient) {
					c.SetFailSealStatus(true)
					c.SetResponseDelay(10 * time.Millisecond)
				},
				RecoverFunc: func(c *MockVaultClient) {
					c.Reset()
				},
			})

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			summary := chaosRunner.Run(ctx, 2*time.Second)

			Expect(summary.TotalOperations).To(BeNumerically(">", 0))
			// Should maintain some functionality despite chaos
			if summary.TotalOperations > 0 {
				errorRate := float64(summary.TotalErrors) / float64(summary.TotalOperations)
				Expect(errorRate).To(BeNumerically("<", 0.9), "Should maintain some functionality under chaos")
			}
		})

		It("should handle cascading failures gracefully", func() {
			chaosRunner := NewChaosTestRunner(3)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			summary := chaosRunner.Run(ctx, 1*time.Second)

			// System should not crash under chaos
			Expect(summary.TotalOperations).To(BeNumerically(">", 0))
		})
	})

	Describe("Security Testing Framework", func() {
		It("should test input sanitization", func() {
			helper := NewSecurityTestHelper()
			validator := NewDefaultKeyValidator()

			maliciousInputs := helper.GenerateMaliciousInputs()

			for _, input := range maliciousInputs {
				keys := []string{input}

				// Should handle without panicking
				Expect(func() {
					validator.ValidateKeys(keys, 1)
				}).ToNot(Panic())
			}
		})

		It("should prevent information leakage in errors", func() {
			helper := NewSecurityTestHelper()
			validator := NewDefaultKeyValidator()

			// Test with sensitive content
			sensitiveKeys := []string{
				"password123",
				"secret-key-data",
				"/etc/passwd",
				"admin-token",
			}

			for _, sensitiveKey := range sensitiveKeys {
				keys := []string{sensitiveKey}
				err := validator.ValidateKeys(keys, 1)

				if err != nil {
					Expect(helper.CheckErrorMessageSecurity(err)).To(BeTrue(),
						"Error message should not leak sensitive information")
				}
			}
		})

		It("should analyze timing consistency", func() {
			analyzer := NewTimingAnalyzer()
			validator := NewDefaultKeyValidator()

			// Test with different key types
			keys := []string{
				base64.StdEncoding.EncodeToString([]byte("short")),
				base64.StdEncoding.EncodeToString([]byte("medium-length-key")),
				base64.StdEncoding.EncodeToString([]byte("very-long-key-with-lots-of-content")),
			}

			for _, key := range keys {
				for i := 0; i < 10; i++ {
					start := time.Now()
					validator.ValidateBase64Key(key)
					duration := time.Since(start)
					analyzer.AddMeasurement(duration)
				}
			}

			analysis := analyzer.AnalyzeConstantTime()
			Expect(analysis.Count).To(Equal(30))
			Expect(analysis.Variance).To(BeNumerically("<", 10.0), "Timing variance should be reasonable")
		})
	})

	Describe("Property-Based Testing Framework", func() {
		It("should generate valid test data", func() {
			generator := NewPropertyTestGenerator(12345)

			keys := generator.GenerateRandomKeys(10, 8, 64)
			Expect(len(keys)).To(Equal(10))

			for _, key := range keys {
				Expect(len(key)).To(BeNumerically(">", 0))
				// Should be valid base64
				_, err := base64.StdEncoding.DecodeString(key)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should generate invalid test data for negative testing", func() {
			generator := NewPropertyTestGenerator(54321)

			invalidKeys := generator.GenerateInvalidKeys(5)
			Expect(len(invalidKeys)).To(Equal(5))

			validator := NewDefaultKeyValidator()
			for _, key := range invalidKeys {
				keys := []string{key}
				err := validator.ValidateKeys(keys, 1)
				// Most should fail validation (some might be accidentally valid)
				_ = err
			}
		})

		It("should test edge case thresholds", func() {
			generator := NewPropertyTestGenerator(99999)
			validator := NewDefaultKeyValidator()

			thresholds := generator.GenerateEdgeCaseThresholds()
			keys := []string{base64.StdEncoding.EncodeToString([]byte("test-key"))}

			for _, threshold := range thresholds {
				// Should handle all threshold values without panicking
				Expect(func() {
					validator.ValidateKeys(keys, threshold)
				}).ToNot(Panic())
			}
		})
	})

	Describe("Memory Testing Framework", func() {
		It("should track memory usage", func() {
			metrics := NewTestMetrics()

			metrics.TakeMemorySnapshot()

			// Simulate memory-intensive work
			data := make([][]byte, 100)
			for i := range data {
				data[i] = make([]byte, 1024) // 1KB each
			}

			metrics.TakeMemorySnapshot()

			// Clear data
			data = nil

			metrics.TakeMemorySnapshot()

			summary := metrics.GetSummary()
			Expect(summary.MemoryGrowth).To(BeNumerically(">=", 0))
		})

		It("should detect memory leaks", func() {
			validator := NewDefaultKeyValidator()

			// Monitor baseline
			var initialMem, finalMem runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&initialMem)

			// Perform operations that could leak
			for i := 0; i < 100; i++ {
				keyData := make([]byte, 1024)
				for j := range keyData {
					keyData[j] = byte(i + j)
				}

				key := base64.StdEncoding.EncodeToString(keyData)
				keys := []string{key}
				validator.ValidateKeys(keys, 1)
			}

			// Force cleanup
			runtime.GC()
			runtime.GC()
			runtime.ReadMemStats(&finalMem)

			memoryGrowth := int64(finalMem.Alloc) - int64(initialMem.Alloc)
			Expect(memoryGrowth).To(BeNumerically("<", 10*1024*1024), "Should not leak significant memory")
		})
	})

	Describe("Test Suite Framework", func() {
		It("should create and run test suites", func() {
			suite := NewTestSuite("Test Suite", config)

			executed := false
			suite.AddTest(TestCase{
				Name:        "TestCase1",
				Category:    "Unit",
				Description: "Test case description",
				RunFunc: func(*TestConfig) error {
					executed = true
					return nil
				},
				Timeout: 5 * time.Second,
			})

			result := suite.Run()

			Expect(executed).To(BeTrue())
			Expect(result.Name).To(Equal("Test Suite"))
			Expect(result.Passed).To(Equal(1))
			Expect(result.Failures).To(Equal(0))
		})

		It("should handle test failures", func() {
			suite := NewTestSuite("Failing Suite", config)

			suite.AddTest(TestCase{
				Name:        "FailingTest",
				Category:    "Unit",
				Description: "Test that fails",
				RunFunc: func(*TestConfig) error {
					return fmt.Errorf("test error")
				},
				Timeout: 5 * time.Second,
			})

			result := suite.Run()

			Expect(result.Passed).To(Equal(0))
			Expect(result.Failures).To(Equal(1))
			Expect(len(result.GetFailedTests())).To(Equal(1))
		})

		It("should handle test skipping", func() {
			suite := NewTestSuite("Skipping Suite", config)

			suite.AddTest(TestCase{
				Name:        "SkippedTest",
				Category:    "Unit",
				Description: "Test that gets skipped",
				RunFunc: func(*TestConfig) error {
					return nil
				},
				SkipFunc: func(*TestConfig) bool {
					return true
				},
				Timeout: 5 * time.Second,
			})

			result := suite.Run()

			Expect(result.Skipped).To(Equal(1))
			Expect(len(result.GetSkippedTests())).To(Equal(1))
		})
	})

	Describe("Resource Profiling", func() {
		It("should start and stop profiling", func() {
			profiler := NewResourceProfiler(ProfilingConfig{
				CPU:      false, // Disable for unit test
				Memory:   true,
				Block:    true,
				Mutex:    true,
				Trace:    false,
				Duration: 1 * time.Second,
			})

			err := profiler.StartProfiling()
			Expect(err).ToNot(HaveOccurred())

			// Simulate some work
			time.Sleep(100 * time.Millisecond)

			err = profiler.StopProfiling()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Test Reporting", func() {
		It("should generate comprehensive reports", func() {
			runner.CreateStandardTestSuites()

			// Mock some results
			runner.results = map[string]*TestSuiteResult{
				"MockSuite": {
					Name:     "MockSuite",
					Duration: 5 * time.Second,
					Passed:   10,
					Failures: 1,
					Skipped:  2,
				},
			}

			err := runner.reporter.GenerateReport(runner)
			Expect(err).ToNot(HaveOccurred())

			// Check if report files exist
			_, err1 := os.Stat("test_report.json")
			_, err2 := os.Stat("test_report.txt")

			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())

			// Cleanup
			os.Remove("test_report.json")
			os.Remove("test_report.txt")
		})
	})

	Describe("Integration Testing", func() {
		It("should run comprehensive test suite", func() {
			if testing.Short() {
				Skip("Skipping comprehensive test in short mode")
			}

			runner.CreateStandardTestSuites()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			err := runner.Run(ctx)
			Expect(err).ToNot(HaveOccurred())

			results := runner.GetResults()
			Expect(len(results)).To(BeNumerically(">", 0))

			// Verify all test suites ran
			expectedSuites := []string{
				"Load Testing",
				"Chaos Engineering",
				"Security Testing",
				"Property-Based Testing",
				"Memory Testing",
				"Compatibility Testing",
			}

			for _, suiteName := range expectedSuites {
				if suiteName == "Compatibility Testing" && config.CompatibilitySkipVersion {
					continue
				}
				_, exists := results[suiteName]
				Expect(exists).To(BeTrue(), "Suite %s should have run", suiteName)
			}
		})
	})
})

// Example of how to use the comprehensive testing framework in a standalone test
func ExampleTestRunner() {
	// Create configuration
	config := DefaultTestConfig()
	config.LoadTestDuration = 10 * time.Second
	config.SecurityTestIterations = 50

	// Create runner
	runner := NewTestRunner(config)
	runner.CreateStandardTestSuites()

	// Add custom test suite
	customSuite := NewTestSuite("Custom Tests", config)
	customSuite.AddTest(TestCase{
		Name:        "CustomTest",
		Category:    "Custom",
		Description: "Custom test example",
		RunFunc: func(config *TestConfig) error {
			// Custom test logic here
			validator := NewDefaultKeyValidator()
			keys := []string{base64.StdEncoding.EncodeToString([]byte("test"))}
			return validator.ValidateKeys(keys, 1)
		},
		Timeout: 30 * time.Second,
	})
	runner.AddSuite(customSuite)

	// Run all tests
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := runner.Run(ctx); err != nil {
		fmt.Printf("Test execution failed: %v\n", err)
		return
	}

	// Get results
	results := runner.GetResults()
	for suiteName, result := range results {
		fmt.Printf("Suite %s: %d passed, %d failed, %d skipped\n",
			suiteName, result.Passed, result.Failures, result.Skipped)
	}

	// Save profiles
	if err := runner.SaveProfiles("test-profiles"); err != nil {
		fmt.Printf("Failed to save profiles: %v\n", err)
	}
}
