package vault

import (
	"context"
	"encoding/base64"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Vault Unsealing Flow", func() {
	var (
		validator   KeyValidator
		mockMetrics *MockClientMetrics
		strategy    UnsealStrategy
	)

	BeforeEach(func() {
		validator = NewDefaultKeyValidator()
		mockMetrics = NewMockClientMetrics()
		strategy = NewDefaultUnsealStrategy(validator, mockMetrics)
	})

	Describe("Successful Unsealing", func() {
		Context("when vault is sealed and keys are valid", func() {
			It("should successfully unseal vault", func() {
				factory := NewMockClientFactory()
				mockClient, err := factory.NewClient("https://vault.example.com:8200", false, 0)
				Expect(err).ToNot(HaveOccurred())

				mock := factory.GetClient("https://vault.example.com:8200")
				mock.SetSealed(true)
				mock.unsealThreshold = 3

				validKeys := []string{
					base64.StdEncoding.EncodeToString([]byte("unseal-key-1")),
					base64.StdEncoding.EncodeToString([]byte("unseal-key-2")),
					base64.StdEncoding.EncodeToString([]byte("unseal-key-3")),
				}

				result, err := strategy.Unseal(context.Background(), mockClient, validKeys, 3)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.Sealed).To(BeFalse())

				attempts := mockMetrics.GetUnsealAttempts()
				Expect(len(attempts)).To(BeNumerically(">", 0))
				Expect(attempts[0].Success).To(BeTrue())
			})

			It("should handle partial key submission", func() {
				factory := NewMockClientFactory()
				mockClient, err := factory.NewClient("https://vault.example.com:8200", false, 0)
				Expect(err).ToNot(HaveOccurred())

				mock := factory.GetClient("https://vault.example.com:8200")
				mock.SetSealed(true)
				mock.unsealThreshold = 2

				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
				}

				result, err := strategy.Unseal(context.Background(), mockClient, keys, 2)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.Sealed).To(BeFalse())
			})
		})

		Context("when vault is already unsealed", func() {
			It("should handle pre-unsealed vault gracefully", func() {
				factory := NewMockClientFactory()
				mockClient, err := factory.NewClient("https://vault.example.com:8200", false, 0)
				Expect(err).ToNot(HaveOccurred())

				mock := factory.GetClient("https://vault.example.com:8200")
				mock.SetSealed(false)

				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				result, err := strategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Sealed).To(BeFalse())
			})
		})
	})

	Describe("Retry Strategy", func() {
		Context("when using retry strategy", func() {
			It("should retry on transient failures", func() {
				baseStrategy := NewDefaultUnsealStrategy(validator, mockMetrics)
				retryPolicy := &DefaultRetryPolicy{
					maxAttempts: 3,
					baseDelay:   1 * time.Millisecond,
					maxDelay:    5 * time.Millisecond,
				}
				retryStrategy := NewRetryUnsealStrategy(baseStrategy, retryPolicy)

				mockClient := NewMockVaultClient()
				mockClient.SetSealed(true)
				mockClient.SetFailSealStatus(true)

				go func() {
					time.Sleep(2 * time.Millisecond)
					mockClient.SetFailSealStatus(false)
					mockClient.SetSealed(false)
				}()

				keys := []string{base64.StdEncoding.EncodeToString([]byte("retry-key"))}

				result, err := retryStrategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Sealed).To(BeFalse())

				Expect(mockClient.GetCallCount("GetSealStatus")).To(BeNumerically(">", 1))
			})

			It("should respect maximum retry attempts", func() {
				baseStrategy := NewDefaultUnsealStrategy(validator, mockMetrics)
				retryPolicy := &DefaultRetryPolicy{
					maxAttempts: 2,
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
				Expect(mockClient.GetCallCount("GetSealStatus")).To(Equal(2))
			})
		})
	})

	Describe("Parallel Strategy", func() {
		Context("when using parallel strategy", func() {
			It("should delegate to base strategy for single instance", func() {
				parallelStrategy := NewParallelUnsealStrategy(strategy, 5)

				factory := NewMockClientFactory()
				mockClient, err := factory.NewClient("https://vault.example.com:8200", false, 0)
				Expect(err).ToNot(HaveOccurred())

				mock := factory.GetClient("https://vault.example.com:8200")
				mock.SetSealed(true)
				mock.unsealThreshold = 1

				keys := []string{base64.StdEncoding.EncodeToString([]byte("parallel-key"))}

				result, err := parallelStrategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Sealed).To(BeFalse())
			})

			It("should handle zero concurrency by setting default", func() {
				parallelStrategy := NewParallelUnsealStrategy(strategy, 0)
				Expect(parallelStrategy).ToNot(BeNil())

				parallelStrategy = NewParallelUnsealStrategy(strategy, -1)
				Expect(parallelStrategy).ToNot(BeNil())
			})
		})
	})

	Describe("Key Submission Logic", func() {
		Context("when submitting keys", func() {
			It("should submit only required number of keys", func() {
				factory := NewMockClientFactory()
				mockClient, err := factory.NewClient("https://vault.example.com:8200", false, 0)
				Expect(err).ToNot(HaveOccurred())

				mock := factory.GetClient("https://vault.example.com:8200")
				mock.SetSealed(true)
				mock.unsealThreshold = 2

				moreKeysThanNeeded := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
					base64.StdEncoding.EncodeToString([]byte("key3")),
					base64.StdEncoding.EncodeToString([]byte("key4")),
				}

				result, err := strategy.Unseal(context.Background(), mockClient, moreKeysThanNeeded, 2)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.Sealed).To(BeFalse())
			})

			It("should stop submitting keys once unsealed", func() {
				factory := NewMockClientFactory()
				mockClient, err := factory.NewClient("https://vault.example.com:8200", false, 0)
				Expect(err).ToNot(HaveOccurred())

				mock := factory.GetClient("https://vault.example.com:8200")
				mock.SetSealed(true)
				mock.unsealThreshold = 1 // Will unseal after first key

				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
				}

				result, err := strategy.Unseal(context.Background(), mockClient, keys, 2)

				Expect(err).ToNot(HaveOccurred())
				Expect(result.Sealed).To(BeFalse())
			})
		})
	})

	Describe("Metrics Recording", func() {
		Context("when unsealing operations complete", func() {
			It("should record successful attempts", func() {
				factory := NewMockClientFactory()
				mockClient, err := factory.NewClient("https://vault.example.com:8200", false, 0)
				Expect(err).ToNot(HaveOccurred())

				mock := factory.GetClient("https://vault.example.com:8200")
				mock.SetSealed(false) // Already unsealed

				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				_, err = strategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).ToNot(HaveOccurred())

				attempts := mockMetrics.GetUnsealAttempts()
				Expect(len(attempts)).To(Equal(1))
				Expect(attempts[0].Success).To(BeTrue())
			})

			It("should record failed attempts", func() {
				factory := NewMockClientFactory()
				mockClient, err := factory.NewClient("https://vault.example.com:8200", false, 0)
				Expect(err).ToNot(HaveOccurred())

				mock := factory.GetClient("https://vault.example.com:8200")
				mock.SetSealed(true)
				mock.SetFailUnseal(true) // This will cause Unseal to fail during key submission

				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				_, err = strategy.Unseal(context.Background(), mockClient, keys, 1)
				Expect(err).To(HaveOccurred())

				attempts := mockMetrics.GetUnsealAttempts()
				Expect(len(attempts)).To(Equal(1))
				Expect(attempts[0].Success).To(BeFalse())
			})
		})
	})
})
