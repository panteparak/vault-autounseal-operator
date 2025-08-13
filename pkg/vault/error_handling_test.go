package vault

import (
	"context"
	"encoding/base64"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Error Handling and Recovery", func() {
	var (
		validator KeyValidator
		strategy  UnsealStrategy
	)

	BeforeEach(func() {
		validator = NewDefaultKeyValidator()
		strategy = NewDefaultUnsealStrategy(validator, nil)
	})

	Describe("Connection Failures", func() {
		Context("when vault connection fails", func() {
			It("should handle vault connection failures gracefully", func() {
				mockClient := NewMockVaultClient()
				mockClient.SetFailSealStatus(true)

				keys := []string{base64.StdEncoding.EncodeToString([]byte("test-key"))}

				result, err := strategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(IsVaultError(err)).To(BeTrue())
			})

			It("should provide meaningful error messages for connection failures", func() {
				mockClient := NewMockVaultClient()
				mockClient.SetFailSealStatus(true)

				keys := []string{base64.StdEncoding.EncodeToString([]byte("test-key"))}

				_, err := strategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("seal status"))
			})
		})
	})

	Describe("Context Cancellation", func() {
		Context("when context is canceled", func() {
			It("should respect context cancellation", func() {
				mockClient := NewMockVaultClient()
				mockClient.SetSealed(true)
				mockClient.SetResponseDelay(100 * time.Millisecond)

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
				defer cancel()

				keys := []string{base64.StdEncoding.EncodeToString([]byte("test-key"))}

				_, err := strategy.Unseal(ctx, mockClient, keys, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context"))
			})

			It("should handle context cancellation during key submission", func() {
				factory := NewMockClientFactory()
				mockClient, err := factory.NewClient("https://vault.example.com:8200", false, 0)
				Expect(err).ToNot(HaveOccurred())

				mock := factory.GetClient("https://vault.example.com:8200")
				mock.SetSealed(true)
				mock.SetResponseDelay(50 * time.Millisecond)
				mock.unsealThreshold = 3

				ctx, cancel := context.WithCancel(context.Background())

				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
					base64.StdEncoding.EncodeToString([]byte("key3")),
				}

				go func() {
					time.Sleep(25 * time.Millisecond)
					cancel()
				}()

				_, err = strategy.Unseal(ctx, mockClient, keys, 3)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context"))
			})
		})
	})

	Describe("Retry Exhaustion", func() {
		Context("when retries are exhausted", func() {
			It("should handle persistent failures with retry exhaustion", func() {
				mockMetrics := NewMockClientMetrics()
				baseStrategy := NewDefaultUnsealStrategy(validator, mockMetrics)

				retryPolicy := &DefaultRetryPolicy{
					maxAttempts: 3,
					baseDelay:   1 * time.Millisecond,
					maxDelay:    3 * time.Millisecond,
				}
				retryStrategy := NewRetryUnsealStrategy(baseStrategy, retryPolicy)

				mockClient := NewMockVaultClient()
				mockClient.SetSealed(true)
				mockClient.SetFailSealStatus(true)

				keys := []string{base64.StdEncoding.EncodeToString([]byte("fail-key"))}

				result, err := retryStrategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed after"))
				Expect(mockClient.GetCallCount("GetSealStatus")).To(Equal(3))
			})

			It("should record failed attempts in metrics", func() {
				mockMetrics := NewMockClientMetrics()
				baseStrategy := NewDefaultUnsealStrategy(validator, mockMetrics)

				factory := NewMockClientFactory()
				mockClient, err := factory.NewClient("https://vault.example.com:8200", false, 0)
				Expect(err).ToNot(HaveOccurred())

				mock := factory.GetClient("https://vault.example.com:8200")
				mock.SetSealed(true)
				mock.SetFailUnseal(true) // This will cause Unseal to fail during key submission

				keys := []string{base64.StdEncoding.EncodeToString([]byte("fail-key"))}

				_, err = baseStrategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).To(HaveOccurred())

				attempts := mockMetrics.GetUnsealAttempts()
				Expect(len(attempts)).To(Equal(1))
				Expect(attempts[0].Success).To(BeFalse())
			})
		})
	})

	Describe("Validation Errors", func() {
		Context("when input validation fails", func() {
			It("should handle validation errors before attempting unsealing", func() {
				mockClient := NewMockVaultClient()
				mockClient.SetSealed(true)

				invalidKeys := []string{"not-base64!@#"}

				result, err := strategy.Unseal(context.Background(), mockClient, invalidKeys, 1)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("validation failed"))

				Expect(mockClient.GetCallCount("GetSealStatus")).To(Equal(0))
			})

			It("should not leak sensitive information in validation errors", func() {
				mockClient := NewMockVaultClient()
				const testSensitiveKey = "secret-admin-password"
				sensitiveKey := testSensitiveKey

				result, err := strategy.Unseal(context.Background(), mockClient, []string{sensitiveKey}, 1)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())

				errorMsg := err.Error()
				Expect(errorMsg).ToNot(ContainSubstring(sensitiveKey))
				Expect(errorMsg).To(ContainSubstring("[REDACTED]"))
			})
		})
	})

	Describe("Network and Transport Errors", func() {
		Context("when network issues occur", func() {
			It("should classify retryable vs non-retryable errors", func() {
				vaultErr := NewVaultError("network-error", "unknown", nil, true)
				Expect(IsRetryableError(vaultErr)).To(BeTrue())

				validationErr := NewValidationError("field", "value", "invalid")
				Expect(IsRetryableError(validationErr)).To(BeFalse())

				unsealErr := &UnsealError{
					Endpoint: "test",
					KeyIndex: 0,
					Err:      NewVaultError("temp-error", "unknown", nil, true),
				}
				Expect(IsRetryableError(unsealErr)).To(BeTrue())
			})

			It("should handle timeout errors appropriately", func() {
				retryPolicy := NewDefaultRetryPolicy()

				timeoutErr := NewVaultError("timeout", "unknown", nil, true)
				Expect(retryPolicy.ShouldRetry(timeoutErr, 0)).To(BeTrue())
				Expect(retryPolicy.ShouldRetry(timeoutErr, 2)).To(BeFalse())
			})
		})
	})

	Describe("Error Recovery", func() {
		Context("when errors can be recovered", func() {
			It("should recover from transient failures", func() {
				retryPolicy := &DefaultRetryPolicy{
					maxAttempts: 3,
					baseDelay:   1 * time.Millisecond,
					maxDelay:    5 * time.Millisecond,
				}
				retryStrategy := NewRetryUnsealStrategy(strategy, retryPolicy)

				mockClient := NewMockVaultClient()
				mockClient.SetSealed(true)
				mockClient.SetFailSealStatus(true)

				go func() {
					time.Sleep(2 * time.Millisecond)
					mockClient.SetFailSealStatus(false)
					mockClient.SetSealed(false)
				}()

				keys := []string{base64.StdEncoding.EncodeToString([]byte("recovery-key"))}

				result, err := retryStrategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Sealed).To(BeFalse())
			})

			It("should use exponential backoff for retries", func() {
				retryPolicy := NewDefaultRetryPolicy()

				delay1 := retryPolicy.NextDelay(0)
				delay2 := retryPolicy.NextDelay(1)
				delay3 := retryPolicy.NextDelay(2)

				Expect(delay2).To(BeNumerically(">", delay1))
				Expect(delay3).To(BeNumerically(">", delay2))
			})

			It("should cap retry delays at maximum", func() {
				retryPolicy := &DefaultRetryPolicy{
					maxAttempts: 10,
					baseDelay:   1 * time.Second,
					maxDelay:    5 * time.Second,
				}

				largeAttempt := retryPolicy.NextDelay(10)
				Expect(largeAttempt).To(Equal(5 * time.Second))
			})
		})
	})

	Describe("Error Propagation", func() {
		Context("when handling error chains", func() {
			It("should preserve error context through the call stack", func() {
				mockClient := NewMockVaultClient()
				mockClient.SetSealed(true)
				mockClient.SetFailUnseal(true)

				keys := []string{base64.StdEncoding.EncodeToString([]byte("chain-key"))}

				_, err := strategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).To(HaveOccurred())

				unsealErr, ok := err.(*UnsealError)
				Expect(ok).To(BeTrue())
				Expect(unsealErr.KeyIndex).To(Equal(0))
				Expect(unsealErr.Endpoint).ToNot(BeEmpty())
			})

			It("should handle nested error wrapping", func() {
				baseErr := NewVaultError("base-error", "test-endpoint", nil, false)
				wrappedErr := &UnsealError{
					Endpoint: "test-endpoint",
					KeyIndex: 1,
					Err:      baseErr,
				}

				Expect(IsVaultError(wrappedErr.Err)).To(BeTrue())
				Expect(wrappedErr.Error()).To(ContainSubstring("key index 1"))
				Expect(wrappedErr.Error()).To(ContainSubstring("test-endpoint"))
			})
		})
	})
})
