package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStrategy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Strategy Suite")
}

var _ = Describe("DefaultUnsealStrategy", func() {
	var strategy *DefaultUnsealStrategy
	var mockClient *MockVaultClient
	var mockValidator *DefaultKeyValidator
	var mockMetrics *MockClientMetrics
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		mockValidator = NewDefaultKeyValidator()
		mockMetrics = NewMockClientMetrics()
		strategy = NewDefaultUnsealStrategy(mockValidator, mockMetrics)
		mockClient = NewMockVaultClient()
	})

	Describe("Unseal", func() {
		Context("with already unsealed vault", func() {
			BeforeEach(func() {
				mockClient.SetSealed(false)
			})

			It("should return immediately without submitting keys", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				result, err := strategy.Unseal(ctx, mockClient, keys, 1)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.Sealed).To(BeFalse())
				Expect(mockClient.GetCallCount("GetSealStatus")).To(Equal(1))
				Expect(mockClient.GetCallCount("Unseal")).To(Equal(0))
			})

			It("should record successful unseal attempt in metrics", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				_, err := strategy.Unseal(ctx, mockClient, keys, 1)

				Expect(err).ToNot(HaveOccurred())
				attempts := mockMetrics.GetUnsealAttempts()
				Expect(len(attempts)).To(Equal(1))
				Expect(attempts[0].Success).To(BeTrue())
			})
		})

		Context("with sealed vault", func() {
			BeforeEach(func() {
				mockClient.SetSealed(true)
			})

			It("should attempt to unseal with provided keys", func() {
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
					base64.StdEncoding.EncodeToString([]byte("key3")),
				}

				result, err := strategy.Unseal(ctx, mockClient, keys, 3)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.Sealed).To(BeFalse()) // Mock unseals after threshold
				Expect(mockClient.GetCallCount("Unseal")).To(Equal(1))
			})

			It("should only use keys up to threshold", func() {
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
					base64.StdEncoding.EncodeToString([]byte("key3")),
					base64.StdEncoding.EncodeToString([]byte("key4")),
					base64.StdEncoding.EncodeToString([]byte("key5")),
				}

				_, err := strategy.Unseal(ctx, mockClient, keys, 3)

				Expect(err).ToNot(HaveOccurred())
				submittedKeys := mockClient.GetSubmittedKeys()
				Expect(len(submittedKeys)).To(Equal(3))
			})
		})

		Context("with validation errors", func() {
			It("should reject empty keys", func() {
				_, err := strategy.Unseal(ctx, mockClient, []string{}, 1)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validation failed"))
				Expect(mockClient.GetCallCount("GetSealStatus")).To(Equal(0))
			})

			It("should reject invalid threshold", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				_, err := strategy.Unseal(ctx, mockClient, keys, 0)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validation failed"))
			})

			It("should reject invalid base64 keys", func() {
				keys := []string{"invalid-base64!@#"}

				_, err := strategy.Unseal(ctx, mockClient, keys, 1)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validation failed"))
			})
		})

		Context("with seal status errors", func() {
			BeforeEach(func() {
				mockClient.SetFailSealStatus(true)
			})

			It("should return error when seal status check fails", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				_, err := strategy.Unseal(ctx, mockClient, keys, 1)

				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&VaultError{}))
			})
		})

		Context("with context cancellation", func() {
			It("should handle cancelled context", func() {
				cancelledCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				_, err := strategy.Unseal(cancelledCtx, mockClient, keys, 1)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context"))
			})

			It("should handle context timeout", func() {
				timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
				defer cancel()

				mockClient.SetResponseDelay(10 * time.Millisecond)
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				_, err := strategy.Unseal(timeoutCtx, mockClient, keys, 1)

				Expect(err).To(HaveOccurred())
			})
		})
	})
})

var _ = Describe("RetryUnsealStrategy", func() {
	var retryStrategy *RetryUnsealStrategy
	var baseStrategy *DefaultUnsealStrategy
	var mockClient *MockVaultClient
	var retryPolicy *DefaultRetryPolicy
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = NewMockVaultClient()
		baseStrategy = NewDefaultUnsealStrategy(NewDefaultKeyValidator(), nil)
		retryPolicy = NewDefaultRetryPolicy()
		retryPolicy.maxAttempts = 3
		retryPolicy.baseDelay = 1 * time.Millisecond // Fast for testing
		retryStrategy = NewRetryUnsealStrategy(baseStrategy, retryPolicy)
	})

	Describe("Unseal with retry logic", func() {
		Context("when base strategy succeeds immediately", func() {
			BeforeEach(func() {
				mockClient.SetSealed(false) // Already unsealed
			})

			It("should succeed on first attempt", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				result, err := retryStrategy.Unseal(ctx, mockClient, keys, 1)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(mockClient.GetCallCount("GetSealStatus")).To(Equal(1))
			})
		})

		Context("when base strategy fails with retryable error", func() {
			BeforeEach(func() {
				mockClient.SetSealed(true)
				mockClient.SetFailSealStatus(true) // This will cause retryable errors
			})

			It("should retry up to max attempts", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				_, err := retryStrategy.Unseal(ctx, mockClient, keys, 1)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed after 3 attempts"))
				// Should have tried multiple times
				Expect(mockClient.GetCallCount("GetSealStatus")).To(BeNumerically(">=", 3))
			})

			It("should succeed if error resolves before max attempts", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				// Make it fail twice, then succeed
				go func() {
					time.Sleep(2 * time.Millisecond)
					mockClient.SetFailSealStatus(false)
					mockClient.SetSealed(false)
				}()

				result, err := retryStrategy.Unseal(ctx, mockClient, keys, 1)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
			})
		})

		Context("with context cancellation during retry", func() {
			It("should stop retrying when context is cancelled", func() {
				cancelCtx, cancel := context.WithCancel(ctx)
				mockClient.SetFailSealStatus(true)
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				go func() {
					time.Sleep(5 * time.Millisecond)
					cancel()
				}()

				_, err := retryStrategy.Unseal(cancelCtx, mockClient, keys, 1)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context"))
			})
		})
	})
})

var _ = Describe("DefaultRetryPolicy", func() {
	var policy *DefaultRetryPolicy

	BeforeEach(func() {
		policy = NewDefaultRetryPolicy()
	})

	Describe("ShouldRetry", func() {
		Context("with retryable errors", func() {
			It("should retry vault errors marked as retryable", func() {
				retryableErr := NewVaultError("test", "endpoint", fmt.Errorf("connection failed"), true)

				should := policy.ShouldRetry(retryableErr, 0)
				Expect(should).To(BeTrue())
			})

			It("should retry connection errors marked as retryable", func() {
				connErr := &ConnectionError{
					Endpoint:  "test",
					Err:       fmt.Errorf("connection failed"),
					Retryable: true,
				}

				should := policy.ShouldRetry(connErr, 0)
				Expect(should).To(BeTrue())
			})
		})

		Context("with non-retryable errors", func() {
			It("should not retry vault errors marked as non-retryable", func() {
				nonRetryableErr := NewVaultError("test", "endpoint", fmt.Errorf("auth failed"), false)

				should := policy.ShouldRetry(nonRetryableErr, 0)
				Expect(should).To(BeFalse())
			})

			It("should not retry validation errors", func() {
				validationErr := NewValidationError("test", "value", "invalid")

				should := policy.ShouldRetry(validationErr, 0)
				Expect(should).To(BeFalse())
			})

			It("should not retry generic errors", func() {
				genericErr := fmt.Errorf("generic error")

				should := policy.ShouldRetry(genericErr, 0)
				Expect(should).To(BeFalse())
			})
		})

		Context("with attempt limits", func() {
			It("should not retry after max attempts", func() {
				retryableErr := NewVaultError("test", "endpoint", fmt.Errorf("connection failed"), true)

				should := policy.ShouldRetry(retryableErr, policy.MaxAttempts()-1)
				Expect(should).To(BeFalse())
			})

			It("should retry before max attempts", func() {
				retryableErr := NewVaultError("test", "endpoint", fmt.Errorf("connection failed"), true)

				should := policy.ShouldRetry(retryableErr, policy.MaxAttempts()-2)
				Expect(should).To(BeTrue())
			})
		})
	})

	Describe("NextDelay", func() {
		It("should implement exponential backoff", func() {
			delay0 := policy.NextDelay(0)
			delay1 := policy.NextDelay(1)
			delay2 := policy.NextDelay(2)

			Expect(delay1).To(BeNumerically(">", delay0))
			Expect(delay2).To(BeNumerically(">", delay1))
		})

		It("should respect maximum delay", func() {
			// Test with high attempt number
			delay := policy.NextDelay(10)

			Expect(delay).To(BeNumerically("<=", policy.maxDelay))
		})

		It("should start with base delay", func() {
			delay := policy.NextDelay(0)

			Expect(delay).To(Equal(policy.baseDelay))
		})
	})

	Describe("MaxAttempts", func() {
		It("should return configured max attempts", func() {
			maxAttempts := policy.MaxAttempts()

			Expect(maxAttempts).To(Equal(policy.maxAttempts))
		})
	})
})

var _ = Describe("Strategy Integration", func() {
	Context("with real-world scenarios", func() {
		var mockClient *MockVaultClient
		var strategy UnsealStrategy
		var ctx context.Context

		BeforeEach(func() {
			ctx = context.Background()
			mockClient = NewMockVaultClient()

			// Set up a retry strategy with fast retries for testing
			baseStrategy := NewDefaultUnsealStrategy(NewDefaultKeyValidator(), NewMockClientMetrics())
			retryPolicy := &DefaultRetryPolicy{
				maxAttempts: 3,
				baseDelay:   1 * time.Millisecond,
				maxDelay:    10 * time.Millisecond,
			}
			strategy = NewRetryUnsealStrategy(baseStrategy, retryPolicy)
		})

		It("should handle partial unsealing scenarios", func() {
			// Simulate a 3-of-5 Shamir setup
			mockClient.sealStatusResp.T = 3
			keys := []string{
				base64.StdEncoding.EncodeToString([]byte("key1")),
				base64.StdEncoding.EncodeToString([]byte("key2")),
				base64.StdEncoding.EncodeToString([]byte("key3")),
				base64.StdEncoding.EncodeToString([]byte("key4")),
				base64.StdEncoding.EncodeToString([]byte("key5")),
			}

			result, err := strategy.Unseal(ctx, mockClient, keys, 3)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Sealed).To(BeFalse())
			Expect(mockClient.GetSubmittedKeys()).To(HaveLen(3))
		})

		It("should handle network intermittency", func() {
			keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

			// Simulate network failure then recovery
			mockClient.SetFailSealStatus(true)

			go func() {
				time.Sleep(5 * time.Millisecond)
				mockClient.SetFailSealStatus(false)
				mockClient.SetSealed(false)
			}()

			result, err := strategy.Unseal(ctx, mockClient, keys, 1)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Sealed).To(BeFalse())
		})

		It("should provide detailed error information on persistent failures", func() {
			keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}
			mockClient.SetFailSealStatus(true)

			_, err := strategy.Unseal(ctx, mockClient, keys, 1)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed after"))
			Expect(err.Error()).To(ContainSubstring("attempts"))
		})
	})
})
