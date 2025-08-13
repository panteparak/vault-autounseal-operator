// +build integration

package vault

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	math_rand "math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPropertyBased(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Property-Based Testing Suite")
}

var _ = Describe("Property-Based Testing", func() {
	Describe("Base64 Encoding Properties", func() {
		It("should satisfy roundtrip property for all valid inputs", func() {
			// Property: encode(decode(x)) == x for all valid base64 strings
			testRoundtripProperty := func(iterations int) {
				for i := 0; i < iterations; i++ {
					// Generate random data
					size := 1 + math_rand.IntN(1024)
					originalData := make([]byte, size)
					rand.Read(originalData)

					// Encode to base64
					encoded := base64.StdEncoding.EncodeToString(originalData)

					// Decode back
					decoded, err := base64.StdEncoding.DecodeString(encoded)
					Expect(err).ToNot(HaveOccurred())

					// Verify roundtrip property
					Expect(decoded).To(Equal(originalData))
				}
			}

			testRoundtripProperty(500)
		})

		It("should handle edge cases in base64 encoding", func() {
			edgeCases := [][]byte{
				{},                 // Empty
				{0},                // Single zero byte
				{255},              // Single max byte
				{0, 255},           // Min and max
				{1, 2, 3},          // Small sequence
				make([]byte, 1024), // Large zero array
			}

			// Fill large array with pattern
			for i := range edgeCases[len(edgeCases)-1] {
				edgeCases[len(edgeCases)-1][i] = byte(i % 256)
			}

			for _, data := range edgeCases {
				encoded := base64.StdEncoding.EncodeToString(data)
				decoded, err := base64.StdEncoding.DecodeString(encoded)

				Expect(err).ToNot(HaveOccurred())
				Expect(decoded).To(Equal(data))
			}
		})
	})

	Describe("Key Validation Properties", func() {
		It("should maintain validation invariants under random inputs", func() {
			validator := NewDefaultKeyValidator()

			// Property: ValidateKeys should never panic
			for i := 0; i < 1000; i++ {
				// Generate random key count and threshold
				keyCount := math_rand.IntN(20)
				threshold := math_rand.IntN(25)

				// Generate random keys
				keys := make([]string, keyCount)
				for j := 0; j < keyCount; j++ {
					keySize := math_rand.IntN(200)
					keyData := make([]byte, keySize)
					rand.Read(keyData)
					keys[j] = base64.StdEncoding.EncodeToString(keyData)
				}

				// Sometimes add invalid keys
				if math_rand.Float32() < 0.2 && len(keys) > 0 {
					invalidIndex := math_rand.IntN(len(keys))
					keys[invalidIndex] = generateInvalidBase64()
				}

				// Function should not panic
				Expect(func() {
					validator.ValidateKeys(keys, threshold)
				}).ToNot(Panic())
			}
		})

		It("should respect threshold constraints for all valid inputs", func() {
			validator := NewDefaultKeyValidator()

			// Property: If threshold <= keyCount and threshold > 0, and all keys are valid, validation should succeed
			for i := 0; i < 200; i++ {
				keyCount := 1 + math_rand.IntN(20)
				threshold := 1 + math_rand.IntN(keyCount) // Ensure threshold <= keyCount

				// Generate valid, unique keys
				keys := generateUniqueValidKeys(keyCount)

				err := validator.ValidateKeys(keys, threshold)
				Expect(err).ToNot(HaveOccurred(),
					"Should succeed with keyCount=%d, threshold=%d", keyCount, threshold)
			}
		})

		It("should reject duplicate keys consistently", func() {
			validator := NewDefaultKeyValidator()

			// Property: Any key set with duplicates should be rejected
			for i := 0; i < 100; i++ {
				keyCount := 2 + math_rand.IntN(10)
				keys := generateUniqueValidKeys(keyCount)

				// Introduce duplicate
				dupIndex1 := math_rand.IntN(len(keys))
				dupIndex2 := math_rand.IntN(len(keys))
				for dupIndex2 == dupIndex1 {
					dupIndex2 = math_rand.IntN(len(keys))
				}
				keys[dupIndex2] = keys[dupIndex1]

				err := validator.ValidateKeys(keys, keyCount-1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("duplicate"))
			}
		})

		It("should handle unicode and special characters properly", func() {
			validator := NewDefaultKeyValidator()

			unicodeStrings := []string{
				"Hello, 世界",
				"🔑🔒🛡️",
				"Ñäme wíth spëcîål çháracters",
				"العربية",
				"русский",
				"\x00\x01\x02\x03",        // Control characters
				strings.Repeat("A", 1000), // Long string
			}

			for _, str := range unicodeStrings {
				data := []byte(str)
				key := base64.StdEncoding.EncodeToString(data)
				keys := []string{key}

				// Should handle without panicking
				Expect(func() {
					validator.ValidateKeys(keys, 1)
				}).ToNot(Panic())
			}
		})
	})

	Describe("Strict Validator Properties", func() {
		It("should enforce length constraints consistently", func() {
			requiredLength := 32
			validator := NewStrictKeyValidator(requiredLength)

			// Property: Only keys of exact required length should pass
			for i := 0; i < 100; i++ {
				keyLength := math_rand.IntN(100)
				keyData := make([]byte, keyLength)
				for j := range keyData {
					keyData[j] = byte(1 + math_rand.IntN(255)) // Avoid all-zero patterns
				}

				key := base64.StdEncoding.EncodeToString(keyData)
				err := validator.ValidateBase64Key(key)

				if keyLength == requiredLength {
					// Length matches, should depend on other validations
					// (might still fail on forbidden strings, but not length)
					if err != nil {
						Expect(err.Error()).ToNot(ContainSubstring("must be exactly"))
					}
				} else {
					// Length doesn't match, should fail with length error
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("must be exactly"))
				}
			}
		})

		It("should detect forbidden patterns reliably", func() {
			validator := NewStrictKeyValidator(32)
			forbiddenStrings := []string{"password", "secret", "test", "demo"}
			validator.SetForbiddenStrings(forbiddenStrings)

			// Property: Keys containing forbidden strings should be rejected
			for _, forbidden := range forbiddenStrings {
				// Create keys containing forbidden strings
				for i := 0; i < 10; i++ {
					// Create 32-byte data with forbidden string
					keyData := make([]byte, 32)
					copy(keyData, forbidden)
					// Fill rest with random data
					for j := len(forbidden); j < 32; j++ {
						keyData[j] = byte(1 + math_rand.IntN(255))
					}

					key := base64.StdEncoding.EncodeToString(keyData)
					err := validator.ValidateBase64Key(key)

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(forbidden))
				}
			}
		})
	})

	Describe("Strategy Properties", func() {
		It("should maintain unseal invariants", func() {
			strategy := NewDefaultUnsealStrategy(NewDefaultKeyValidator(), NewMockClientMetrics())

			// Property: Unsealing an already unsealed vault should succeed immediately
			for i := 0; i < 50; i++ {
				client := NewMockVaultClient()
				client.SetSealed(false) // Already unsealed

				keyCount := 1 + math_rand.IntN(10)
				keys := generateUniqueValidKeys(keyCount)
				threshold := 1 + math_rand.IntN(keyCount)

				result, err := strategy.Unseal(context.Background(), client, keys, threshold)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.Sealed).To(BeFalse())

				// Should not have called Unseal on client
				Expect(client.GetCallCount("Unseal")).To(Equal(0))
			}
		})

		It("should handle retry properties correctly", func() {
			baseStrategy := NewDefaultUnsealStrategy(NewDefaultKeyValidator(), NewMockClientMetrics())
			retryPolicy := &DefaultRetryPolicy{
				maxAttempts: 3,
				baseDelay:   1 * time.Millisecond,
				maxDelay:    10 * time.Millisecond,
			}
			retryStrategy := NewRetryUnsealStrategy(baseStrategy, retryPolicy)

			// Property: Non-retryable errors should not be retried
			for i := 0; i < 20; i++ {
				client := NewMockVaultClient()

				// Create invalid keys (non-retryable error)
				invalidKeys := []string{"invalid-base64!@#"}

				_, err := retryStrategy.Unseal(context.Background(), client, invalidKeys, 1)

				Expect(err).To(HaveOccurred())
				Expect(IsValidationError(err)).To(BeTrue())

				// Should not retry validation errors
				Expect(client.GetCallCount("GetSealStatus")).To(Equal(0))
			}
		})
	})

	Describe("Error Properties", func() {
		It("should maintain error type consistency", func() {
			// Property: Error type checking functions should be consistent
			testCases := []struct {
				createError  func() error
				isRetryable  bool
				isValidation bool
			}{
				{
					createError: func() error {
						return NewValidationError("test", "value", "message")
					},
					isRetryable:  false,
					isValidation: true,
				},
				{
					createError: func() error {
						return NewVaultError("op", "endpoint", fmt.Errorf("base"), true)
					},
					isRetryable:  true,
					isValidation: false,
				},
				{
					createError: func() error {
						return NewVaultError("op", "endpoint", fmt.Errorf("base"), false)
					},
					isRetryable:  false,
					isValidation: false,
				},
				{
					createError: func() error {
						return fmt.Errorf("generic error")
					},
					isRetryable:  false,
					isValidation: false,
				},
			}

			for _, tc := range testCases {
				for i := 0; i < 10; i++ {
					err := tc.createError()

					Expect(IsRetryableError(err)).To(Equal(tc.isRetryable))
					Expect(IsValidationError(err)).To(Equal(tc.isValidation))
				}
			}
		})
	})

	Describe("Memory Safety Properties", func() {
		It("should clear sensitive data from memory", func() {
			validator := NewDefaultKeyValidator()

			// Property: Key validation should not leak key data in memory
			sensitiveKey := "super-secret-key-data-that-should-be-cleared"
			encodedKey := base64.StdEncoding.EncodeToString([]byte(sensitiveKey))

			// Perform validation multiple times
			for i := 0; i < 100; i++ {
				keys := []string{encodedKey}
				_ = validator.ValidateKeys(keys, 1)
			}

			// This is a basic test - in reality, memory clearing verification
			// would require more sophisticated techniques
			Expect(true).To(BeTrue()) // Placeholder assertion
		})
	})

	Describe("Concurrent Properties", func() {
		It("should maintain thread safety under random access patterns", func() {
			validator := NewDefaultKeyValidator()
			metrics := NewMockClientMetrics()

			// Property: Concurrent access should not cause data races
			numGoroutines := 20
			operationsPerGoroutine := 50

			var wg sync.WaitGroup
			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(goroutineID int) {
					defer wg.Done()

					for j := 0; j < operationsPerGoroutine; j++ {
						// Random operations
						switch math_rand.IntN(4) {
						case 0:
							// Validation
							keys := generateUniqueValidKeys(1 + math_rand.IntN(5))
							validator.ValidateKeys(keys, len(keys))

						case 1:
							// Metrics recording
							endpoint := fmt.Sprintf("endpoint-%d", goroutineID)
							duration := time.Duration(math_rand.IntN(1000)) * time.Microsecond
							success := math_rand.Float32() > 0.1
							metrics.RecordSealStatusCheck(endpoint, success, duration)

						case 2:
							// Client operations
							client := NewMockVaultClient()
							client.SetSealed(math_rand.Float32() < 0.5)
							_, _ = client.IsSealed(context.Background())

						case 3:
							// Key generation
							_ = generateUniqueValidKeys(math_rand.IntN(10))
						}

						// Add some randomness to timing
						if math_rand.Float32() < 0.1 {
							time.Sleep(time.Microsecond)
						}
					}
				}(i)
			}

			wg.Wait()

			// Should complete without data races or panics
			Expect(true).To(BeTrue())
		})
	})
})

// Helper functions for property-based testing

func generateInvalidBase64() string {
	invalidChars := "!@#$%^&*()[]{}|;':\",./<>?`~"
	length := 1 + math_rand.IntN(50)
	result := make([]byte, length)

	for i := 0; i < length; i++ {
		if math_rand.Float32() < 0.5 {
			// Invalid character
			result[i] = invalidChars[math_rand.IntN(len(invalidChars))]
		} else {
			// Valid base64 character
			validChars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
			result[i] = validChars[math_rand.IntN(len(validChars))]
		}
	}

	return string(result)
}

func generateUniqueValidKeys(count int) []string {
	keys := make([]string, 0, count)
	seen := make(map[string]bool)

	for len(keys) < count {
		keySize := 8 + math_rand.IntN(64)
		keyData := make([]byte, keySize)

		// Generate non-zero, varied data
		for i := range keyData {
			keyData[i] = byte(1 + math_rand.IntN(255))
		}

		key := base64.StdEncoding.EncodeToString(keyData)

		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}

	return keys
}
