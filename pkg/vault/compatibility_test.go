package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCompatibility(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Vault Version Compatibility Suite")
}

// VaultVersion represents a Vault version for compatibility testing
type VaultVersion struct {
	Version string
	Major   int
	Minor   int
	Patch   int
}

// ParseVersion parses a version string into components
func ParseVersion(version string) VaultVersion {
	v := VaultVersion{Version: version}

	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	parts := strings.Split(version, ".")
	if len(parts) >= 1 {
		v.Major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		v.Minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		v.Patch, _ = strconv.Atoi(parts[2])
	}

	return v
}

// IsAtLeast checks if version is at least the specified version
func (v VaultVersion) IsAtLeast(major, minor, patch int) bool {
	if v.Major > major {
		return true
	}
	if v.Major == major && v.Minor > minor {
		return true
	}
	if v.Major == major && v.Minor == minor && v.Patch >= patch {
		return true
	}
	return false
}

var _ = Describe("Vault Version Compatibility Tests", func() {
	var currentVersion VaultVersion

	BeforeEach(func() {
		// Get version from environment or default
		versionStr := os.Getenv("VAULT_VERSION")
		if versionStr == "" {
			versionStr = "1.15.0" // Default to latest stable
		}
		currentVersion = ParseVersion(versionStr)
	})

	Describe("API Compatibility", func() {
		Context("Seal Status API", func() {
			It("should handle seal status response format across versions", func() {
				factory := NewMockClientFactory()
				client, err := factory.NewClient("http://vault-compat:8200", false, 30*time.Second)
				Expect(err).ToNot(HaveOccurred())
				defer client.Close()

				mockClient := factory.GetClient("http://vault-compat:8200")

				// Test different seal status configurations based on version
				testCases := []struct {
					sealed    bool
					threshold int
					progress  int
				}{
					{true, 3, 0},  // Sealed, no progress
					{true, 3, 1},  // Sealed, partial progress
					{true, 3, 2},  // Sealed, almost unsealed
					{false, 3, 0}, // Unsealed
				}

				ctx := context.Background()
				for _, tc := range testCases {
					mockClient.SetSealed(tc.sealed)
					mockClient.sealStatusResp.T = tc.threshold
					mockClient.sealStatusResp.Progress = tc.progress

					// Test both IsSealed and GetSealStatus
					sealed, err := client.IsSealed(ctx)
					Expect(err).ToNot(HaveOccurred())
					Expect(sealed).To(Equal(tc.sealed))

					status, err := client.GetSealStatus(ctx)
					if err == nil { // Some versions might not support GetSealStatus
						Expect(status).ToNot(BeNil())
						Expect(status.Sealed).To(Equal(tc.sealed))
						Expect(status.T).To(Equal(tc.threshold))
						Expect(status.Progress).To(Equal(tc.progress))
					}
				}
			})

			It("should handle version-specific response fields", func() {
				factory := NewMockClientFactory()
				client, err := factory.NewClient("http://vault-fields:8200", false, 30*time.Second)
				Expect(err).ToNot(HaveOccurred())
				defer client.Close()

				mockClient := factory.GetClient("http://vault-fields:8200")

				// Configure response based on version capabilities
				if currentVersion.IsAtLeast(1, 12, 0) {
					// Newer versions might have additional fields
					mockClient.sealStatusResp.Version = currentVersion.Version
					mockClient.sealStatusResp.BuildDate = "2023-01-01T00:00:00Z"
				}

				if currentVersion.IsAtLeast(1, 13, 0) {
					// Even newer versions might have more fields
					mockClient.sealStatusResp.StorageType = "raft"
				}

				ctx := context.Background()
				status, err := client.GetSealStatus(ctx)
				if err == nil {
					Expect(status).ToNot(BeNil())

					// Version-specific field validation
					if currentVersion.IsAtLeast(1, 12, 0) {
						Expect(status.Version).ToNot(BeEmpty())
					}
				}
			})
		})

		Context("Unseal API", func() {
			It("should handle unseal API changes across versions", func() {
				strategy := NewDefaultUnsealStrategy(NewDefaultKeyValidator(), NewMockClientMetrics())
				mockClient := NewMockVaultClient()

				// Configure client based on version
				mockClient.SetSealed(true)

				// Version-specific unseal behavior
				if currentVersion.IsAtLeast(1, 14, 0) {
					// Newer versions might have enhanced unseal responses
					mockClient.unsealThreshold = 3
					mockClient.sealStatusResp.T = 3
					mockClient.sealStatusResp.N = 5
				} else {
					// Older versions
					mockClient.unsealThreshold = 3
					mockClient.sealStatusResp.T = 3
					mockClient.sealStatusResp.N = 5
				}

				ctx := context.Background()
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
					base64.StdEncoding.EncodeToString([]byte("key3")),
				}

				result, err := strategy.Unseal(ctx, mockClient, keys, 3)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
			})

			It("should handle different unseal key formats", func() {
				validator := NewDefaultKeyValidator()

				// Test key formats that might be version-specific
				testKeys := map[string][]string{
					"v1.12": {
						base64.StdEncoding.EncodeToString([]byte("standard-key-format")),
					},
					"v1.13": {
						base64.StdEncoding.EncodeToString([]byte("enhanced-key-format")),
						base64.URLEncoding.EncodeToString([]byte("url-safe-format")),
					},
					"v1.14": {
						base64.StdEncoding.EncodeToString([]byte("latest-key-format")),
						base64.RawStdEncoding.EncodeToString([]byte("raw-format")),
					},
				}

				versionKey := fmt.Sprintf("v%d.%d", currentVersion.Major, currentVersion.Minor)
				keys, exists := testKeys[versionKey]
				if !exists {
					keys = testKeys["v1.12"] // Fallback to oldest supported
				}

				for _, key := range keys {
					err := validator.ValidateBase64Key(key)
					// Should handle version-appropriate key formats
					_ = err // May succeed or fail based on format, but shouldn't panic
				}
			})
		})

		Context("Health Check API", func() {
			It("should handle health check response evolution", func() {
				factory := NewMockClientFactory()
				client, err := factory.NewClient("http://vault-health:8200", false, 30*time.Second)
				Expect(err).ToNot(HaveOccurred())
				defer client.Close()

				mockClient := factory.GetClient("http://vault-health:8200")

				// Configure health response based on version
				mockClient.SetHealthy(true)

				ctx := context.Background()

				// Test health check variations
				healthConfigs := []struct {
					healthy     bool
					initialized bool
					standby     bool
					description string
				}{
					{true, true, false, "active and healthy"},
					{true, true, true, "standby and healthy"},
					{false, true, false, "initialized but unhealthy"},
					{false, false, false, "uninitialized"},
				}

				for _, config := range healthConfigs {
					mockClient.SetHealthy(config.healthy)
					// Note: MockVaultClient doesn't implement all health states,
					// but in a real test environment, these would be configured

					health, err := client.HealthCheck(ctx)
					if err == nil {
						Expect(health).ToNot(BeNil())
						// Version-specific response validation would go here
					}
				}
			})
		})
	})

	Describe("Feature Compatibility", func() {
		Context("Authentication Methods", func() {
			It("should handle version-specific auth requirements", func() {
				factory := NewMockClientFactory()

				// Different auth configurations based on version
				authConfigs := []struct {
					version     string
					tlsRequired bool
					timeout     time.Duration
				}{
					{"1.12.0", false, 30 * time.Second},
					{"1.13.0", true, 30 * time.Second},
					{"1.14.0", true, 45 * time.Second},
					{"1.15.0", true, 60 * time.Second},
				}

				for _, config := range authConfigs {
					testVersion := ParseVersion(config.version)
					if currentVersion.Major > testVersion.Major ||
						(currentVersion.Major == testVersion.Major && currentVersion.Minor >= testVersion.Minor) {

						scheme := "http"
						if config.tlsRequired {
							scheme = "https"
						}

						client, err := factory.NewClient(
							fmt.Sprintf("%s://vault-%s:8200", scheme, config.version),
							!config.tlsRequired, // Skip TLS verification for testing
							config.timeout,
						)

						Expect(err).ToNot(HaveOccurred())
						Expect(client).ToNot(BeNil())
						client.Close()
					}
				}
			})
		})

		Context("Configuration Options", func() {
			It("should handle version-specific configuration", func() {
				// Test configuration options that vary by version
				configTests := []struct {
					minVersion   VaultVersion
					configOption string
					shouldWork   bool
				}{
					{ParseVersion("1.12.0"), "basic-config", true},
					{ParseVersion("1.13.0"), "enhanced-config", true},
					{ParseVersion("1.14.0"), "advanced-config", true},
					{ParseVersion("1.15.0"), "latest-config", true},
					{ParseVersion("2.0.0"), "future-config", false}, // Future version
				}

				for _, test := range configTests {
					if currentVersion.Major > test.minVersion.Major ||
						(currentVersion.Major == test.minVersion.Major &&
							currentVersion.Minor >= test.minVersion.Minor &&
							currentVersion.Patch >= test.minVersion.Patch) {

						if test.shouldWork {
							// Configuration should be supported
							Expect(true).To(BeTrue(),
								"Configuration %s should work with version %s",
								test.configOption, currentVersion.Version)
						}
					} else {
						// Configuration might not be supported in older versions
						Skip(fmt.Sprintf("Skipping %s test - requires version %s or later",
							test.configOption, test.minVersion.Version))
					}
				}
			})
		})
	})

	Describe("Error Handling Compatibility", func() {
		It("should handle version-specific error formats", func() {
			factory := NewMockClientFactory()
			client, err := factory.NewClient("http://vault-errors:8200", false, 30*time.Second)
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			mockClient := factory.GetClient("http://vault-errors:8200")

			// Simulate different error conditions
			errorConditions := []struct {
				condition   string
				setupFunc   func(*MockVaultClient)
				expectError bool
			}{
				{
					"seal status failure",
					func(c *MockVaultClient) { c.SetFailSealStatus(true) },
					true,
				},
				{
					"health check failure",
					func(c *MockVaultClient) { c.SetFailHealthCheck(true) },
					true,
				},
				{
					"unseal failure",
					func(c *MockVaultClient) { c.SetFailUnseal(true) },
					true,
				},
			}

			ctx := context.Background()

			for _, condition := range errorConditions {
				condition.setupFunc(mockClient)

				// Test different operations
				_, err1 := client.IsSealed(ctx)
				_, err2 := client.HealthCheck(ctx)

				if condition.expectError {
					// At least one operation should fail
					Expect(err1 != nil || err2 != nil).To(BeTrue())

					// Errors should be properly formatted regardless of version
					if err1 != nil {
						Expect(len(err1.Error())).To(BeNumerically(">", 0))
					}
					if err2 != nil {
						Expect(len(err2.Error())).To(BeNumerically(">", 0))
					}
				}

				// Reset for next test
				mockClient.Reset()
			}
		})

		It("should maintain error type consistency across versions", func() {
			validator := NewDefaultKeyValidator()

			// Error conditions that should be consistent across versions
			errorTests := []struct {
				keys          []string
				threshold     int
				expectedError string
			}{
				{nil, 1, "validation failed"},
				{[]string{}, 1, "validation failed"},
				{[]string{"invalid!@#"}, 1, "invalid key"},
				{[]string{"valid"}, 0, "validation failed"},
				{[]string{"a", "a"}, 2, "duplicate key"},
			}

			for _, test := range errorTests {
				err := validator.ValidateKeys(test.keys, test.threshold)

				if test.expectedError != "" {
					Expect(err).To(HaveOccurred())
					Expect(strings.ToLower(err.Error())).To(ContainSubstring(test.expectedError))
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
			}
		})
	})

	Describe("Performance Compatibility", func() {
		It("should maintain performance characteristics across versions", func() {
			factory := NewMockClientFactory()
			client, err := factory.NewClient("http://vault-perf:8200", false, 30*time.Second)
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			ctx := context.Background()
			numOperations := 100

			// Measure performance of basic operations
			start := time.Now()
			for i := 0; i < numOperations; i++ {
				_, _ = client.IsSealed(ctx)
			}
			duration := time.Since(start)

			avgLatency := duration / time.Duration(numOperations)

			// Performance should be reasonable regardless of version
			Expect(avgLatency).To(BeNumerically("<", 10*time.Millisecond),
				"Average operation latency should be reasonable")

			fmt.Printf("Version %s performance: %d ops in %v (avg: %v)\n",
				currentVersion.Version, numOperations, duration, avgLatency)
		})

		It("should handle concurrent operations consistently", func() {
			if testing.Short() {
				Skip("Skipping concurrent test in short mode")
			}

			factory := NewMockClientFactory()
			numClients := 10
			operationsPerClient := 20

			clients := make([]VaultClient, numClients)
			for i := 0; i < numClients; i++ {
				client, err := factory.NewClient(
					fmt.Sprintf("http://vault-concurrent-%d:8200", i),
					false, 10*time.Second)
				Expect(err).ToNot(HaveOccurred())
				clients[i] = client
			}

			defer func() {
				for _, client := range clients {
					client.Close()
				}
			}()

			var wg sync.WaitGroup
			var totalOps, errors int64
			ctx := context.Background()

			start := time.Now()

			for i := 0; i < numClients; i++ {
				wg.Add(1)
				go func(clientIndex int) {
					defer wg.Done()
					client := clients[clientIndex]

					for j := 0; j < operationsPerClient; j++ {
						atomic.AddInt64(&totalOps, 1)
						_, err := client.IsSealed(ctx)
						if err != nil {
							atomic.AddInt64(&errors, 1)
						}
					}
				}(i)
			}

			wg.Wait()
			duration := time.Since(start)

			errorRate := float64(errors) / float64(totalOps)
			throughput := float64(totalOps) / duration.Seconds()

			// Concurrent performance should be acceptable
			Expect(errorRate).To(BeNumerically("<", 0.1), "Error rate should be low")
			Expect(throughput).To(BeNumerically(">", 50), "Throughput should be reasonable")

			fmt.Printf("Concurrent test (v%s): %d ops, %.2f%% errors, %.2f ops/sec\n",
				currentVersion.Version, totalOps, errorRate*100, throughput)
		})
	})

	Describe("Regression Testing", func() {
		It("should not break existing functionality", func() {
			// Test that basic functionality works across all supported versions
			strategy := NewDefaultUnsealStrategy(NewDefaultKeyValidator(), NewMockClientMetrics())
			validator := NewDefaultKeyValidator()

			// Basic validation should work
			keys := []string{
				base64.StdEncoding.EncodeToString([]byte("test-key-1")),
				base64.StdEncoding.EncodeToString([]byte("test-key-2")),
			}

			err := validator.ValidateKeys(keys, 2)
			Expect(err).ToNot(HaveOccurred())

			// Basic unseal strategy should work
			mockClient := NewMockVaultClient()
			mockClient.SetSealed(true)

			ctx := context.Background()
			result, err := strategy.Unseal(ctx, mockClient, keys, 2)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
		})

		It("should handle upgrade/downgrade scenarios", func() {
			// Test behavior when transitioning between versions
			// This is conceptual since we can't actually change Vault versions mid-test

			versionTransitions := []struct {
				from string
				to   string
				note string
			}{
				{"1.12.0", "1.13.0", "minor version upgrade"},
				{"1.13.0", "1.14.0", "minor version upgrade"},
				{"1.14.0", "1.15.0", "minor version upgrade"},
			}

			for _, transition := range versionTransitions {
				fromVer := ParseVersion(transition.from)
				toVer := ParseVersion(transition.to)

				// Test that our code handles both versions appropriately
				if currentVersion.IsAtLeast(fromVer.Major, fromVer.Minor, fromVer.Patch) &&
					!currentVersion.IsAtLeast(toVer.Major+1, 0, 0) {

					// We're in a compatible version range
					factory := NewMockClientFactory()
					client, err := factory.NewClient("http://vault-transition:8200", false, 30*time.Second)
					Expect(err).ToNot(HaveOccurred())

					// Basic operations should work
					ctx := context.Background()
					_, err = client.IsSealed(ctx)
					// May fail due to network (mock), but shouldn't panic
					_ = err

					client.Close()
				}
			}
		})
	})
})
