// +build integration

package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCRDValidation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRD Validation Integration Suite")
}

// MockCRDConfig represents a mock CRD configuration for testing
type MockCRDConfig struct {
	Name             string            `json:"name"`
	Namespace        string            `json:"namespace"`
	VaultAddress     string            `json:"vaultAddress"`
	UnsealKeys       []string          `json:"unsealKeys"`
	KeyThreshold     int               `json:"keyThreshold"`
	Timeout          string            `json:"timeout"`
	RetryAttempts    int               `json:"retryAttempts"`
	Labels           map[string]string `json:"labels,omitempty"`
	Annotations      map[string]string `json:"annotations,omitempty"`
	HealthCheckPath  string            `json:"healthCheckPath,omitempty"`
	TLSConfig        *TLSConfig        `json:"tlsConfig,omitempty"`
}

type TLSConfig struct {
	Enabled            bool   `json:"enabled"`
	CertificatePath    string `json:"certificatePath,omitempty"`
	PrivateKeyPath     string `json:"privateKeyPath,omitempty"`
	CACertificatePath  string `json:"caCertificatePath,omitempty"`
	SkipVerify         bool   `json:"skipVerify,omitempty"`
}

// CRDValidator simulates CRD validation logic
type CRDValidator struct {
	strictMode bool
}

func NewCRDValidator(strictMode bool) *CRDValidator {
	return &CRDValidator{strictMode: strictMode}
}

func (v *CRDValidator) ValidateCRD(config *MockCRDConfig) error {
	// Required field validation
	if config.Name == "" {
		return fmt.Errorf("CRD validation failed: name is required")
	}

	if config.Namespace == "" {
		return fmt.Errorf("CRD validation failed: namespace is required")
	}

	if config.VaultAddress == "" {
		return fmt.Errorf("CRD validation failed: vaultAddress is required")
	}

	// URL validation
	if !strings.HasPrefix(config.VaultAddress, "http://") && !strings.HasPrefix(config.VaultAddress, "https://") {
		return fmt.Errorf("CRD validation failed: vaultAddress must start with http:// or https://, got: %s", config.VaultAddress)
	}

	// Unseal keys validation
	if len(config.UnsealKeys) == 0 {
		return fmt.Errorf("CRD validation failed: unsealKeys cannot be empty")
	}

	// Validate each unseal key
	for i, key := range config.UnsealKeys {
		if key == "" {
			return fmt.Errorf("CRD validation failed: unsealKeys[%d] cannot be empty", i)
		}

		// Check if it's valid base64
		if _, err := base64.StdEncoding.DecodeString(key); err != nil {
			return fmt.Errorf("CRD validation failed: unsealKeys[%d] is not valid base64: %v", i, err)
		}

		// In strict mode, check for weak keys
		if v.strictMode {
			if strings.Contains(strings.ToLower(key), "test") {
				return fmt.Errorf("CRD validation failed: unsealKeys[%d] contains 'test' which is not allowed in production", i)
			}

			if strings.Contains(strings.ToLower(key), "demo") {
				return fmt.Errorf("CRD validation failed: unsealKeys[%d] contains 'demo' which is not allowed in production", i)
			}
		}
	}

	// Key threshold validation
	if config.KeyThreshold <= 0 {
		return fmt.Errorf("CRD validation failed: keyThreshold must be positive, got: %d", config.KeyThreshold)
	}

	if config.KeyThreshold > len(config.UnsealKeys) {
		return fmt.Errorf("CRD validation failed: keyThreshold (%d) cannot exceed number of unsealKeys (%d)",
			config.KeyThreshold, len(config.UnsealKeys))
	}

	// Timeout validation
	if config.Timeout != "" {
		if _, err := time.ParseDuration(config.Timeout); err != nil {
			return fmt.Errorf("CRD validation failed: timeout is not a valid duration: %v", err)
		}
	}

	// Retry attempts validation
	if config.RetryAttempts < 0 {
		return fmt.Errorf("CRD validation failed: retryAttempts cannot be negative, got: %d", config.RetryAttempts)
	}

	if config.RetryAttempts > 10 {
		return fmt.Errorf("CRD validation failed: retryAttempts cannot exceed 10, got: %d", config.RetryAttempts)
	}

	// Name validation (Kubernetes naming rules)
	if len(config.Name) > 63 {
		return fmt.Errorf("CRD validation failed: name cannot exceed 63 characters, got: %d", len(config.Name))
	}

	if !isValidKubernetesName(config.Name) {
		return fmt.Errorf("CRD validation failed: name '%s' is not a valid Kubernetes name (must contain only lowercase letters, numbers, and hyphens)", config.Name)
	}

	// Namespace validation
	if !isValidKubernetesName(config.Namespace) {
		return fmt.Errorf("CRD validation failed: namespace '%s' is not a valid Kubernetes namespace name", config.Namespace)
	}

	// Labels validation
	if config.Labels != nil {
		for key, value := range config.Labels {
			if err := validateKubernetesLabel(key, value); err != nil {
				return fmt.Errorf("CRD validation failed: invalid label %s=%s: %v", key, value, err)
			}
		}
	}

	// TLS configuration validation
	if config.TLSConfig != nil && config.TLSConfig.Enabled {
		if config.TLSConfig.CertificatePath == "" && !config.TLSConfig.SkipVerify {
			return fmt.Errorf("CRD validation failed: certificatePath is required when TLS is enabled and skipVerify is false")
		}

		if config.TLSConfig.PrivateKeyPath == "" && !config.TLSConfig.SkipVerify {
			return fmt.Errorf("CRD validation failed: privateKeyPath is required when TLS is enabled and skipVerify is false")
		}
	}

	return nil
}

// Helper functions for validation
func isValidKubernetesName(name string) bool {
	if len(name) == 0 {
		return false
	}

	// Must start and end with alphanumeric
	if !isAlphaNumeric(name[0]) || !isAlphaNumeric(name[len(name)-1]) {
		return false
	}

	// Only lowercase letters, numbers, and hyphens
	for _, char := range name {
		if !isAlphaNumeric(byte(char)) && char != '-' {
			return false
		}
	}

	return true
}

func isAlphaNumeric(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')
}

func validateKubernetesLabel(key, value string) error {
	if len(key) > 63 {
		return fmt.Errorf("key too long: %d characters (max 63)", len(key))
	}

	if len(value) > 63 {
		return fmt.Errorf("value too long: %d characters (max 63)", len(value))
	}

	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	return nil
}

var _ = Describe("CRD Validation Negative Test Cases", func() {
	var (
		runner    *EnhancedIntegrationTestRunner
		config    *IntegrationTestConfig
		validator *CRDValidator
	)

	BeforeEach(func() {
		// Fast configuration for CRD validation tests
		config = &IntegrationTestConfig{
			QuickTimeout:        500 * time.Millisecond, // Very fast for validation
			OperationTimeout:    2 * time.Second,        // Quick validation
			MaxTotalTime:        10 * time.Second,       // Fast total time
			FailureThreshold:    1,                      // Fail immediately for validation
			SuccessThreshold:    1,
			CooldownPeriod:      200 * time.Millisecond,
			HealthCheckInterval: 100 * time.Millisecond,
			MaxUnhealthyTime:    1 * time.Second,        // Don't wait long
			MaxConcurrency:      5,
		}

		runner = NewEnhancedIntegrationTestRunner(config, DebugConfig())
		validator = NewCRDValidator(false) // Start with non-strict mode

		GinkgoWriter.Printf("🧪 Starting CRD Validation Tests (Debug: %s)\n", DebugConfig().String())
	})

	AfterEach(func() {
		if DebugConfig() >= DebugLevelBasic {
			runner.PrintDebugReport()
		}
		runner.Close()
	})

	Describe("Required Field Validation Failures", func() {
		It("should fail fast when required fields are missing", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			invalidConfigs := []struct {
				name        string
				config      *MockCRDConfig
				expectedErr string
			}{
				{
					name:        "missing-name",
					config:      &MockCRDConfig{Namespace: "default", VaultAddress: "https://vault.example.com"},
					expectedErr: "name is required",
				},
				{
					name:        "missing-namespace",
					config:      &MockCRDConfig{Name: "test-vault", VaultAddress: "https://vault.example.com"},
					expectedErr: "namespace is required",
				},
				{
					name:        "missing-vault-address",
					config:      &MockCRDConfig{Name: "test-vault", Namespace: "default"},
					expectedErr: "vaultAddress is required",
				},
				{
					name: "missing-unseal-keys",
					config: &MockCRDConfig{
						Name:         "test-vault",
						Namespace:    "default",
						VaultAddress: "https://vault.example.com",
						UnsealKeys:   []string{},
					},
					expectedErr: "unsealKeys cannot be empty",
				},
			}

			for _, tc := range invalidConfigs {
				err := runner.RunTestWithDebug(ctx, fmt.Sprintf("validate-%s", tc.name), func(testCtx context.Context) error {
					start := time.Now()
					err := validator.ValidateCRD(tc.config)
					duration := time.Since(start)

					// Should fail quickly (validation should be fast)
					Expect(duration).To(BeNumerically("<", 100*time.Millisecond),
						"Validation should be very fast for %s", tc.name)

					return err
				})

				Expect(err).To(HaveOccurred(), "Should fail for %s", tc.name)
				Expect(err.Error()).To(ContainSubstring(tc.expectedErr),
					"Should contain expected error for %s", tc.name)
			}
		})
	})

	Describe("URL Validation Failures", func() {
		It("should fail fast for invalid vault addresses", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			invalidURLs := []struct {
				url         string
				description string
			}{
				{"not-a-url", "plain text"},
				{"ftp://vault.example.com", "wrong protocol"},
				{"file:///etc/passwd", "file protocol"},
				{"javascript:alert('xss')", "javascript injection"},
				{"", "empty string"},
				{"vault.example.com", "missing protocol"},
			}

			for _, tc := range invalidURLs {
				testName := fmt.Sprintf("invalid-url-%s", strings.ReplaceAll(tc.description, " ", "-"))

				err := runner.RunTestWithDebug(ctx, testName, func(testCtx context.Context) error {
					config := &MockCRDConfig{
						Name:         "test-vault",
						Namespace:    "default",
						VaultAddress: tc.url,
						UnsealKeys:   []string{base64.StdEncoding.EncodeToString([]byte("test-key"))},
						KeyThreshold: 1,
					}

					return validator.ValidateCRD(config)
				})

				Expect(err).To(HaveOccurred(), "Should fail for %s: %s", tc.description, tc.url)
				Expect(err.Error()).To(ContainSubstring("vaultAddress must start with http"),
					"Should contain URL validation error for %s", tc.description)
			}
		})
	})

	Describe("Unseal Key Validation Failures", func() {
		It("should fail fast for invalid base64 keys", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			invalidBase64Keys := []string{
				"not-base64!@#$",
				"invalid-chars-äöü",
				"has spaces in it",
				"ends-with-equals==extra",
				"",
				"123", // Too short and invalid
			}

			for i, invalidKey := range invalidBase64Keys {
				testName := fmt.Sprintf("invalid-base64-key-%d", i)

				err := runner.RunTestWithDebug(ctx, testName, func(testCtx context.Context) error {
					config := &MockCRDConfig{
						Name:         "test-vault",
						Namespace:    "default",
						VaultAddress: "https://vault.example.com",
						UnsealKeys:   []string{invalidKey},
						KeyThreshold: 1,
					}

					return validator.ValidateCRD(config)
				})

				Expect(err).To(HaveOccurred(), "Should fail for invalid base64 key: %s", invalidKey)
				Expect(err.Error()).To(ContainSubstring("not valid base64"),
					"Should contain base64 validation error for key: %s", invalidKey)
			}
		})

		It("should fail fast for threshold validation errors", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			thresholdTests := []struct {
				name        string
				keys        []string
				threshold   int
				expectedErr string
			}{
				{
					name:        "threshold-too-high",
					keys:        []string{base64.StdEncoding.EncodeToString([]byte("key1"))},
					threshold:   5,
					expectedErr: "keyThreshold (5) cannot exceed number of unsealKeys (1)",
				},
				{
					name:        "threshold-zero",
					keys:        []string{base64.StdEncoding.EncodeToString([]byte("key1"))},
					threshold:   0,
					expectedErr: "keyThreshold must be positive, got: 0",
				},
				{
					name:        "threshold-negative",
					keys:        []string{base64.StdEncoding.EncodeToString([]byte("key1"))},
					threshold:   -1,
					expectedErr: "keyThreshold must be positive, got: -1",
				},
			}

			for _, tc := range thresholdTests {
				err := runner.RunTestWithDebug(ctx, tc.name, func(testCtx context.Context) error {
					config := &MockCRDConfig{
						Name:         "test-vault",
						Namespace:    "default",
						VaultAddress: "https://vault.example.com",
						UnsealKeys:   tc.keys,
						KeyThreshold: tc.threshold,
					}

					return validator.ValidateCRD(config)
				})

				Expect(err).To(HaveOccurred(), "Should fail for %s", tc.name)
				Expect(err.Error()).To(ContainSubstring(tc.expectedErr),
					"Should contain expected error for %s", tc.name)
			}
		})
	})

	Describe("Kubernetes Naming Validation Failures", func() {
		It("should fail fast for invalid Kubernetes names", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			invalidNames := []struct {
				name        string
				description string
			}{
				{"Test-Vault", "uppercase letters"},
				{"test_vault", "underscores"},
				{"test.vault", "dots"},
				{"-test-vault", "starts with hyphen"},
				{"test-vault-", "ends with hyphen"},
				{"test@vault", "special characters"},
				{strings.Repeat("a", 64), "too long (>63 chars)"},
				{"", "empty"},
				{"123-vault", "starts with number is OK"},
				{"test vault", "spaces"},
			}

			validKey := base64.StdEncoding.EncodeToString([]byte("test-key"))

			for _, tc := range invalidNames {
				testName := fmt.Sprintf("invalid-name-%s", strings.ReplaceAll(tc.description, " ", "-"))

				err := runner.RunTestWithDebug(ctx, testName, func(testCtx context.Context) error {
					config := &MockCRDConfig{
						Name:         tc.name,
						Namespace:    "default",
						VaultAddress: "https://vault.example.com",
						UnsealKeys:   []string{validKey},
						KeyThreshold: 1,
					}

					return validator.ValidateCRD(config)
				})

				// Some names are actually valid (like "123-vault")
				if tc.name == "123-vault" {
					Expect(err).ToNot(HaveOccurred(), "Name %s should be valid", tc.name)
				} else {
					Expect(err).To(HaveOccurred(), "Should fail for %s: %s", tc.description, tc.name)
					if tc.name != "" { // Empty name fails on required field check
						Expect(err.Error()).To(ContainSubstring("not a valid Kubernetes name"),
							"Should contain naming validation error for %s", tc.description)
					}
				}
			}
		})
	})

	Describe("Strict Mode Validation Failures", func() {
		BeforeEach(func() {
			validator = NewCRDValidator(true) // Enable strict mode
		})

		It("should fail fast for production-unsafe configurations", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			unsafeKeys := []string{
				base64.StdEncoding.EncodeToString([]byte("test-key-data")),
				base64.StdEncoding.EncodeToString([]byte("demo-vault-key")),
				base64.StdEncoding.EncodeToString([]byte("testing-123")),
			}

			for i, unsafeKey := range unsafeKeys {
				testName := fmt.Sprintf("unsafe-key-strict-mode-%d", i)

				err := runner.RunTestWithDebug(ctx, testName, func(testCtx context.Context) error {
					config := &MockCRDConfig{
						Name:         "production-vault",
						Namespace:    "production",
						VaultAddress: "https://vault.production.com",
						UnsealKeys:   []string{unsafeKey},
						KeyThreshold: 1,
					}

					return validator.ValidateCRD(config)
				})

				Expect(err).To(HaveOccurred(), "Should fail for unsafe key in strict mode: %s", unsafeKey)
				Expect(err.Error()).To(SatisfyAny(
					ContainSubstring("contains 'test'"),
					ContainSubstring("contains 'demo'"),
				), "Should contain production safety error")
			}
		})
	})

	Describe("Complex Configuration Validation Failures", func() {
		It("should fail fast for invalid timeout durations", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			invalidTimeouts := []string{
				"not-a-duration",
				"30", // Missing unit
				"30x", // Invalid unit
				"-5s", // Negative duration
				"", // Empty is OK, will be skipped
			}

			validKey := base64.StdEncoding.EncodeToString([]byte("valid-key"))

			for _, timeout := range invalidTimeouts {
				if timeout == "" {
					continue // Empty timeout is allowed
				}

				testName := fmt.Sprintf("invalid-timeout-%s", strings.ReplaceAll(timeout, "-", "neg"))

				err := runner.RunTestWithDebug(ctx, testName, func(testCtx context.Context) error {
					config := &MockCRDConfig{
						Name:         "test-vault",
						Namespace:    "default",
						VaultAddress: "https://vault.example.com",
						UnsealKeys:   []string{validKey},
						KeyThreshold: 1,
						Timeout:      timeout,
					}

					return validator.ValidateCRD(config)
				})

				Expect(err).To(HaveOccurred(), "Should fail for invalid timeout: %s", timeout)
				Expect(err.Error()).To(ContainSubstring("not a valid duration"),
					"Should contain duration validation error for: %s", timeout)
			}
		})

		It("should fail fast for invalid retry attempts", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			invalidRetries := []int{-1, -5, 15, 100}
			validKey := base64.StdEncoding.EncodeToString([]byte("valid-key"))

			for _, retries := range invalidRetries {
				testName := fmt.Sprintf("invalid-retries-%d", retries)

				err := runner.RunTestWithDebug(ctx, testName, func(testCtx context.Context) error {
					config := &MockCRDConfig{
						Name:          "test-vault",
						Namespace:     "default",
						VaultAddress:  "https://vault.example.com",
						UnsealKeys:    []string{validKey},
						KeyThreshold:  1,
						RetryAttempts: retries,
					}

					return validator.ValidateCRD(config)
				})

				Expect(err).To(HaveOccurred(), "Should fail for invalid retry attempts: %d", retries)
				if retries < 0 {
					Expect(err.Error()).To(ContainSubstring("cannot be negative"),
						"Should contain negative validation error for: %d", retries)
				} else {
					Expect(err.Error()).To(ContainSubstring("cannot exceed 10"),
						"Should contain max validation error for: %d", retries)
				}
			}
		})
	})

	Describe("TLS Configuration Validation Failures", func() {
		It("should fail fast for incomplete TLS configuration", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			validKey := base64.StdEncoding.EncodeToString([]byte("valid-key"))

			tlsTests := []struct {
				name        string
				tlsConfig   *TLSConfig
				expectedErr string
			}{
				{
					name: "missing-cert-path",
					tlsConfig: &TLSConfig{
						Enabled:         true,
						PrivateKeyPath:  "/path/to/key.pem",
						SkipVerify:      false,
					},
					expectedErr: "certificatePath is required",
				},
				{
					name: "missing-key-path",
					tlsConfig: &TLSConfig{
						Enabled:         true,
						CertificatePath: "/path/to/cert.pem",
						SkipVerify:      false,
					},
					expectedErr: "privateKeyPath is required",
				},
			}

			for _, tc := range tlsTests {
				err := runner.RunTestWithDebug(ctx, tc.name, func(testCtx context.Context) error {
					config := &MockCRDConfig{
						Name:         "test-vault",
						Namespace:    "default",
						VaultAddress: "https://vault.example.com",
						UnsealKeys:   []string{validKey},
						KeyThreshold: 1,
						TLSConfig:    tc.tlsConfig,
					}

					return validator.ValidateCRD(config)
				})

				Expect(err).To(HaveOccurred(), "Should fail for %s", tc.name)
				Expect(err.Error()).To(ContainSubstring(tc.expectedErr),
					"Should contain expected error for %s", tc.name)
			}
		})
	})

	Describe("Performance and Circuit Breaker Validation", func() {
		It("should demonstrate fast validation failure detection", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Test multiple validation failures to trigger circuit breaker
			start := time.Now()

			for i := 0; i < 3; i++ {
				testName := fmt.Sprintf("rapid-validation-failure-%d", i)

				err := runner.RunTestWithDebug(ctx, testName, func(testCtx context.Context) error {
					// Create an invalid config that will fail quickly
					config := &MockCRDConfig{
						Name:         "", // Invalid: missing name
						Namespace:    "",
						VaultAddress: "",
					}

					return validator.ValidateCRD(config)
				})

				Expect(err).To(HaveOccurred())

				// After the failure threshold (1), should fail fast with circuit breaker
				if i >= config.FailureThreshold {
					Expect(err.Error()).To(ContainSubstring("circuit breaker"),
						"Should fail fast with circuit breaker after %d failures", i)
				}
			}

			totalDuration := time.Since(start)

			// All validation failures should complete very quickly
			Expect(totalDuration).To(BeNumerically("<", 2*time.Second),
				"Multiple validation failures should complete quickly")

			GinkgoWriter.Printf("✅ Completed 3 validation failures in %v\n", totalDuration)
		})

		It("should show performance comparison with timing analysis", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			validKey := base64.StdEncoding.EncodeToString([]byte("valid-key"))

			// Test various validation scenarios and measure timing
			validationTests := []struct {
				name   string
				config *MockCRDConfig
				valid  bool
			}{
				{
					name: "valid-minimal-config",
					config: &MockCRDConfig{
						Name:         "valid-vault",
						Namespace:    "default",
						VaultAddress: "https://vault.example.com",
						UnsealKeys:   []string{validKey},
						KeyThreshold: 1,
					},
					valid: true,
				},
				{
					name: "invalid-missing-name",
					config: &MockCRDConfig{
						Namespace:    "default",
						VaultAddress: "https://vault.example.com",
						UnsealKeys:   []string{validKey},
						KeyThreshold: 1,
					},
					valid: false,
				},
				{
					name: "invalid-bad-url",
					config: &MockCRDConfig{
						Name:         "test-vault",
						Namespace:    "default",
						VaultAddress: "not-a-url",
						UnsealKeys:   []string{validKey},
						KeyThreshold: 1,
					},
					valid: false,
				},
			}

			// Reset circuit breaker with new runner for clean test
			testRunner := NewEnhancedIntegrationTestRunner(config, DebugConfig())
			defer testRunner.Close()

			totalStart := time.Now()
			successCount := 0
			errorCount := 0

			for _, tc := range validationTests {
				err := testRunner.RunTestWithDebug(ctx, tc.name, func(testCtx context.Context) error {
					validationStart := time.Now()
					err := validator.ValidateCRD(tc.config)
					validationDuration := time.Since(validationStart)

					// All validation should be very fast (< 50ms)
					Expect(validationDuration).To(BeNumerically("<", 50*time.Millisecond),
						"Validation should be very fast for %s", tc.name)

					return err
				})

				if tc.valid {
					Expect(err).ToNot(HaveOccurred(), "Should succeed for %s", tc.name)
					successCount++
				} else {
					Expect(err).To(HaveOccurred(), "Should fail for %s", tc.name)
					errorCount++
				}
			}

			totalDuration := time.Since(totalStart)

			GinkgoWriter.Printf("📊 Validation Performance Summary:\n")
			GinkgoWriter.Printf("  • Total time: %v\n", totalDuration)
			GinkgoWriter.Printf("  • Successful validations: %d\n", successCount)
			GinkgoWriter.Printf("  • Failed validations: %d\n", errorCount)
			GinkgoWriter.Printf("  • Average per validation: %v\n", totalDuration/time.Duration(len(validationTests)))

			// Verify the framework provides fast feedback
			Expect(totalDuration).To(BeNumerically("<", 1*time.Second),
				"All validation tests should complete in under 1 second")
		})
	})
})
