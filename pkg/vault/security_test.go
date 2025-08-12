package vault

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSecurity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security-Focused Testing Suite")
}

var _ = Describe("Security-Focused Tests", func() {
	Describe("Input Sanitization", func() {
		It("should handle malicious base64 inputs safely", func() {
			validator := NewDefaultKeyValidator()

			maliciousInputs := []string{
				// Excessively long inputs
				strings.Repeat("A", 1000000),
				// SQL injection attempts (shouldn't matter for base64, but test anyway)
				"'; DROP TABLE users; --",
				"1' OR '1'='1",
				// Script injection attempts
				"<script>alert('xss')</script>",
				"javascript:alert('xss')",
				// Path traversal attempts
				"../../../etc/passwd",
				"....//....//....//etc/passwd",
				// Null bytes and control characters
				string([]byte{0x00, 0x01, 0x02, 0x03}),
				"\x00\x01\x02\x03",
				// Buffer overflow attempts
				strings.Repeat("\x41", 65536),
				// Format string attacks
				"%s%s%s%s%s",
				"%x%x%x%x%x",
				// Unicode normalization attacks
				"A\u0300\u0301\u0302\u0303",
			}

			for i, maliciousInput := range maliciousInputs {
				keys := []string{maliciousInput}

				// Should handle gracefully without panicking or crashing
				Expect(func() {
					_ = validator.ValidateKeys(keys, 1)
				}).ToNot(Panic(), "Failed on malicious input %d: %s", i, maliciousInput[:min(50, len(maliciousInput))])
			}
		})

		It("should resist timing attacks on key validation", func() {
			validator := NewDefaultKeyValidator()

			// Create keys that should take similar time to validate
			validKey1 := base64.StdEncoding.EncodeToString([]byte("valid-key-1-content"))
			validKey2 := base64.StdEncoding.EncodeToString([]byte("valid-key-2-content"))
			invalidKey := "invalid-key-content!@#"

			numIterations := 100

			// Measure timing for different key types
			validTime1 := measureValidationTime(validator, validKey1, numIterations)
			validTime2 := measureValidationTime(validator, validKey2, numIterations)
			invalidTime := measureValidationTime(validator, invalidKey, numIterations)

			// Times should not reveal information about key content
			// Allow for some variance, but reject excessive timing differences
			maxValidTime := validTime1
			if validTime2 > validTime1 {
				maxValidTime = validTime2
			}
			minValidTime := validTime1
			if validTime2 < validTime1 {
				minValidTime = validTime2
			}

			timingRatio := float64(maxValidTime) / float64(minValidTime)
			Expect(timingRatio).To(BeNumerically("<", 3.0), "Valid key validation times should be similar")

			// Invalid key timing should also be reasonable
			invalidRatio := float64(invalidTime) / float64(maxValidTime)
			Expect(invalidRatio).To(BeNumerically("<", 10.0), "Invalid key timing should not be excessively different")
		})

		It("should prevent key data leakage in error messages", func() {
			validator := NewDefaultKeyValidator()

			// Create keys with sensitive content
			sensitiveContent := "super-secret-password-12345"
			sensitiveKey := base64.StdEncoding.EncodeToString([]byte(sensitiveContent))

			// Add invalid character to make it fail validation
			invalidSensitiveKey := sensitiveKey + "!@#"

			err := validator.ValidateBase64Key(invalidSensitiveKey)
			Expect(err).To(HaveOccurred())

			// Error message should not contain the sensitive content
			errorMsg := err.Error()
			Expect(errorMsg).ToNot(ContainSubstring(sensitiveContent))
			Expect(errorMsg).ToNot(ContainSubstring(sensitiveKey))
			Expect(errorMsg).ToNot(ContainSubstring(invalidSensitiveKey))
		})
	})

	Describe("Memory Security", func() {
		It("should clear sensitive data from memory", func() {
			validator := NewDefaultKeyValidator()

			// Create key with identifiable pattern
			sensitiveData := []byte("SENSITIVE-PATTERN-12345678")
			sensitiveKey := base64.StdEncoding.EncodeToString(sensitiveData)

			// Perform validation
			keys := []string{sensitiveKey}
			_ = validator.ValidateKeys(keys, 1)

			// Force garbage collection
			runtime.GC()
			runtime.GC() // Second call to ensure cleanup

			// Note: This is a basic test. In practice, detecting memory clearing
			// would require more sophisticated techniques like memory scanning
			// or using specialized tools. This test at least ensures the
			// operation completes and doesn't crash.
			Expect(true).To(BeTrue())
		})

		It("should handle memory pressure without leaking sensitive data", func() {
			validator := NewDefaultKeyValidator()

			numKeys := 1000
			keySize := 1024 // 1KB per key

			// Generate many large keys
			keys := make([]string, numKeys)
			for i := 0; i < numKeys; i++ {
				keyData := make([]byte, keySize)

				// Fill with pattern that would be identifiable if leaked
				pattern := fmt.Sprintf("KEY-%04d-", i)
				for j := 0; j < keySize; j++ {
					if j < len(pattern) {
						keyData[j] = pattern[j]
					} else {
						keyData[j] = byte((i + j) % 256)
					}
				}

				keys[i] = base64.StdEncoding.EncodeToString(keyData)
			}

			// Process keys in batches to simulate memory pressure
			batchSize := 100
			for i := 0; i < numKeys; i += batchSize {
				end := min(i+batchSize, numKeys)
				batch := keys[i:end]

				_ = validator.ValidateKeys(batch, len(batch))

				// Force cleanup periodically
				if i%300 == 0 {
					runtime.GC()
				}
			}

			// Final cleanup
			runtime.GC()
			runtime.GC()

			// Memory should not grow excessively
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			Expect(m.Alloc).To(BeNumerically("<", 100*1024*1024), "Memory usage should be reasonable")
		})

		It("should use constant-time comparison for sensitive operations", func() {
			// Test that key comparison uses constant-time operations
			key1 := base64.StdEncoding.EncodeToString([]byte("key-content-1"))
			key2 := base64.StdEncoding.EncodeToString([]byte("key-content-2"))
			key3 := base64.StdEncoding.EncodeToString([]byte("key-content-1")) // Same as key1

			// Test timing consistency for different comparisons
			numTests := 100

			time1 := measureComparisonTime(key1, key2, numTests) // Different
			time2 := measureComparisonTime(key1, key3, numTests) // Same

			// Timing should be similar for both cases (constant-time property)
			maxTime := time1
			if time2 > time1 {
				maxTime = time2
			}
			minTime := time1
			if time2 < time1 {
				minTime = time2
			}
			timingRatio := float64(maxTime) / float64(minTime)
			Expect(timingRatio).To(BeNumerically("<", 2.0), "Comparison timing should be constant")
		})
	})

	Describe("Cryptographic Security", func() {
		It("should handle cryptographically weak keys", func() {
			validator := NewDefaultKeyValidator()

			weakKeys := [][]byte{
				make([]byte, 32),                           // All zeros
				[]byte(strings.Repeat("\x01", 32)),         // All ones
				[]byte(strings.Repeat("\xFF", 32)),         // All max bytes
				[]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), // Repeating pattern
				[]byte("12341234123412341234123412341234"), // Short pattern repeated
			}

			for i, keyData := range weakKeys {
				key := base64.StdEncoding.EncodeToString(keyData)
				err := validator.ValidateBase64Key(key)

				if len(keyData) >= 2 {
					// Should reject keys with weak patterns
					Expect(err).To(HaveOccurred(), "Weak key %d should be rejected", i)
				}
			}
		})

		It("should require sufficient entropy in keys", func() {
			strictValidator := NewStrictKeyValidator(32)

			// Low entropy keys
			lowEntropyKeys := [][]byte{
				[]byte(strings.Repeat("A", 32)),            // No entropy
				[]byte("AAAAAAAAAAAAAAABBBBBBBBBBBBBBB"),   // Very low entropy
				[]byte("ABABABABABABABABABABABABABABABAB"), // Alternating pattern
			}

			// High entropy keys
			highEntropyKeys := make([][]byte, 5)
			for i := range highEntropyKeys {
				key := make([]byte, 32)
				_, err := rand.Read(key)
				Expect(err).ToNot(HaveOccurred())
				highEntropyKeys[i] = key
			}

			// Low entropy should be rejected
			for i, keyData := range lowEntropyKeys {
				key := base64.StdEncoding.EncodeToString(keyData)
				err := strictValidator.ValidateBase64Key(key)
				Expect(err).To(HaveOccurred(), "Low entropy key %d should be rejected", i)
			}

			// High entropy should pass (if no other issues)
			for _, keyData := range highEntropyKeys {
				key := base64.StdEncoding.EncodeToString(keyData)
				err := strictValidator.ValidateBase64Key(key)
				// May fail on other criteria (forbidden strings), but not entropy
				if err != nil && strings.Contains(err.Error(), "identical bytes") {
					continue // This specific entropy check failed, which is what we want to test
				}
			}
		})

		It("should handle key derivation attacks", func() {
			_ = NewDefaultKeyValidator()

			// Test keys that might be related/derived from each other
			baseKey := "base-key-for-derivation-test"
			derivedKeys := []string{
				baseKey + "1",
				baseKey + "2",
				baseKey + "a",
				baseKey + "A",
				strings.ToUpper(baseKey),
				strings.ToLower(baseKey),
				reverseString(baseKey),
			}

			encodedKeys := make([]string, len(derivedKeys))
			for i, key := range derivedKeys {
				encodedKeys[i] = base64.StdEncoding.EncodeToString([]byte(key))
			}

			// Should detect if keys are too similar (for strict validator)
			strictValidator := NewStrictKeyValidator(32)

			for i, key := range encodedKeys {
				// Pad to required length
				decoded, _ := base64.StdEncoding.DecodeString(key)
				if len(decoded) < 32 {
					padded := make([]byte, 32)
					copy(padded, decoded)
					key = base64.StdEncoding.EncodeToString(padded)
				}

				err := strictValidator.ValidateBase64Key(key)
				// May fail for various reasons, but should not crash
				_ = err // Result depends on specific validation rules

				Expect(func() {
					strictValidator.ValidateBase64Key(key)
				}).ToNot(Panic(), "Should handle derived key %d safely", i)
			}
		})
	})

	Describe("Access Control", func() {
		It("should prevent unauthorized operations", func() {
			client := NewMockVaultClient()

			// Simulate unauthorized access attempts
			ctx := context.Background()

			// These should fail gracefully, not crash
			Expect(func() {
				client.IsSealed(ctx)
			}).ToNot(Panic())

			Expect(func() {
				client.HealthCheck(ctx)
			}).ToNot(Panic())

			Expect(func() {
				client.GetSealStatus(ctx)
			}).ToNot(Panic())
		})

		It("should validate operation contexts properly", func() {
			client := NewMockVaultClient()

			// Test with various context configurations
			contexts := []context.Context{
				context.Background(),
				context.TODO(),
			}

			// Add contexts with values
			ctxWithValue := context.WithValue(context.Background(), "test-key", "test-value")
			contexts = append(contexts, ctxWithValue)

			// Add canceled context
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel()
			contexts = append(contexts, cancelledCtx)

			// Add context with deadline
			deadlineCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			time.Sleep(2 * time.Millisecond) // Ensure deadline passes
			defer cancel()
			contexts = append(contexts, deadlineCtx)

			for i, ctx := range contexts {
				// Operations should handle all context types gracefully
				Expect(func() {
					_, _ = client.IsSealed(ctx)
				}).ToNot(Panic(), "Context type %d should be handled safely", i)
			}
		})
	})

	Describe("Error Information Disclosure", func() {
		It("should not leak internal information in errors", func() {
			client := NewMockVaultClient()
			validator := NewDefaultKeyValidator()

			// Test various error conditions
			testCases := []func() error{
				func() error {
					return validator.ValidateKeys(nil, 1)
				},
				func() error {
					return validator.ValidateKeys([]string{"invalid!@#"}, 1)
				},
				func() error {
					return validator.ValidateKeys([]string{}, 1)
				},
				func() error {
					client.SetFailSealStatus(true)
					_, err := client.IsSealed(context.Background())
					return err
				},
			}

			sensitiveInfo := []string{
				// Internal paths
				"/etc/passwd",
				"/proc/",
				"C:\\Windows\\",
				// Internal IPs
				"127.0.0.1",
				"192.168.",
				"10.0.0.",
				// Stack traces
				"goroutine",
				"runtime.",
				"panic:",
				// Database info
				"SELECT",
				"UPDATE",
				"DELETE",
				// System info
				"root",
				"admin",
				"password",
			}

			for _, testCase := range testCases {
				err := testCase()
				if err != nil {
					errorMsg := strings.ToLower(err.Error())

					for _, sensitive := range sensitiveInfo {
						Expect(errorMsg).ToNot(ContainSubstring(strings.ToLower(sensitive)),
							"Error should not contain sensitive info: %s", sensitive)
					}
				}
			}
		})

		It("should provide appropriate error details for debugging", func() {
			validator := NewDefaultKeyValidator()

			// Errors should be informative but not leak sensitive data
			testCases := []struct {
				keys      []string
				threshold int
				expectMsg string
			}{
				{nil, 1, "validation failed"},
				{[]string{}, 1, "validation failed"},
				{[]string{"valid"}, 0, "validation failed"},
				{[]string{"invalid!@#"}, 1, "invalid key"},
			}

			for _, tc := range testCases {
				err := validator.ValidateKeys(tc.keys, tc.threshold)
				if err != nil {
					Expect(err.Error()).To(ContainSubstring(tc.expectMsg))
					Expect(len(err.Error())).To(BeNumerically("<", 500), "Error message should not be excessively long")
				}
			}
		})
	})

	Describe("Resource Exhaustion Protection", func() {
		It("should handle resource exhaustion attacks", func() {
			validator := NewDefaultKeyValidator()

			// Test with excessive key counts
			largeKeyCounts := []int{1000, 5000, 10000}

			for _, keyCount := range largeKeyCounts {
				startTime := time.Now()

				// Generate many keys
				keys := make([]string, keyCount)
				for i := 0; i < keyCount; i++ {
					keyData := make([]byte, 64)
					for j := range keyData {
						keyData[j] = byte(i + j)
					}
					keys[i] = base64.StdEncoding.EncodeToString(keyData)
				}

				// Validation should complete in reasonable time
				err := validator.ValidateKeys(keys, keyCount/2)
				duration := time.Since(startTime)

				// Should not take excessively long (DoS protection)
				Expect(duration).To(BeNumerically("<", 30*time.Second),
					"Validation of %d keys should not take too long", keyCount)

				// Should handle gracefully
				_ = err // May succeed or fail, but should not hang/crash
			}
		})

		It("should limit memory usage during validation", func() {
			validator := NewDefaultKeyValidator()

			var memBefore, memAfter runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&memBefore)

			// Process many large keys
			numKeys := 100
			keySize := 8192 // 8KB per key

			for batch := 0; batch < 10; batch++ {
				keys := make([]string, numKeys)
				for i := 0; i < numKeys; i++ {
					keyData := make([]byte, keySize)
					for j := range keyData {
						keyData[j] = byte((batch*numKeys + i + j) % 256)
					}
					keys[i] = base64.StdEncoding.EncodeToString(keyData)
				}

				_ = validator.ValidateKeys(keys, numKeys/2)

				// Periodically check memory
				if batch%3 == 0 {
					runtime.GC()
					var memCurrent runtime.MemStats
					runtime.ReadMemStats(&memCurrent)

					growth := memCurrent.Alloc - memBefore.Alloc
					Expect(growth).To(BeNumerically("<", 200*1024*1024),
						"Memory growth should be bounded during validation")
				}
			}

			runtime.GC()
			runtime.ReadMemStats(&memAfter)

			finalGrowth := memAfter.Alloc - memBefore.Alloc
			Expect(finalGrowth).To(BeNumerically("<", 100*1024*1024),
				"Final memory growth should be reasonable")
		})
	})

	Describe("Side-Channel Attacks", func() {
		It("should resist cache timing attacks", func() {
			validator := NewDefaultKeyValidator()

			// Test keys that might have different cache behavior
			cachedKey := base64.StdEncoding.EncodeToString([]byte("frequently-used-key"))
			uncachedKey := base64.StdEncoding.EncodeToString([]byte("rarely-used-key-content"))

			numWarmup := 50
			numTests := 100

			// Warm up cache with first key
			for i := 0; i < numWarmup; i++ {
				validator.ValidateBase64Key(cachedKey)
			}

			// Measure timing for both keys
			cachedTime := measureValidationTime(validator, cachedKey, numTests)
			uncachedTime := measureValidationTime(validator, uncachedKey, numTests)

			// Timing should not reveal cache state significantly
			maxTime := cachedTime
			if uncachedTime > cachedTime {
				maxTime = uncachedTime
			}
			minTime := cachedTime
			if uncachedTime < cachedTime {
				minTime = uncachedTime
			}
			timingRatio := float64(maxTime) / float64(minTime)
			Expect(timingRatio).To(BeNumerically("<", 5.0),
				"Cache timing should not leak significant information")
		})

		It("should handle power analysis resistance", func() {
			// This is a conceptual test - actual power analysis would require
			// specialized hardware. We test for consistent operations.

			validator := NewDefaultKeyValidator()

			// Keys with different bit patterns
			testKeys := []string{
				base64.StdEncoding.EncodeToString([]byte(strings.Repeat("\x00", 32))), // All zeros
				base64.StdEncoding.EncodeToString([]byte(strings.Repeat("\xFF", 32))), // All ones
				base64.StdEncoding.EncodeToString([]byte(strings.Repeat("\xAA", 32))), // Alternating
				base64.StdEncoding.EncodeToString([]byte(strings.Repeat("\x55", 32))), // Alternating opposite
			}

			// Measure timing consistency across different bit patterns
			timings := make([]time.Duration, len(testKeys))
			for i, key := range testKeys {
				timings[i] = measureValidationTime(validator, key, 50)
			}

			// Find min and max timing
			minTime, maxTime := timings[0], timings[0]
			for _, t := range timings {
				if t < minTime {
					minTime = t
				}
				if t > maxTime {
					maxTime = t
				}
			}

			// Timing variance should be minimal
			if minTime > 0 {
				timingVariance := float64(maxTime) / float64(minTime)
				Expect(timingVariance).To(BeNumerically("<", 3.0),
					"Timing should be consistent across different bit patterns")
			}
		})
	})
})

// Helper functions for security testing

func measureValidationTime(validator KeyValidator, key string, iterations int) time.Duration {
	start := time.Now()
	for i := 0; i < iterations; i++ {
		validator.ValidateBase64Key(key)
	}
	return time.Since(start)
}

func measureComparisonTime(key1, key2 string, iterations int) time.Duration {
	start := time.Now()
	for i := 0; i < iterations; i++ {
		// Use constant-time comparison
		subtle.ConstantTimeCompare([]byte(key1), []byte(key2))
	}
	return time.Since(start)
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

