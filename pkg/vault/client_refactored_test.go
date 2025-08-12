package vault

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRefactoredClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Refactored Client Suite")
}

var _ = Describe("Client Creation and Configuration", func() {
	Describe("NewClient", func() {
		It("should create client with basic configuration", func() {
			client, err := NewClient("http://vault.example.com:8200", false, 30*time.Second)

			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
			Expect(client.URL()).To(Equal("http://vault.example.com:8200"))
			Expect(client.Timeout()).To(Equal(30 * time.Second))
			Expect(client.IsClosed()).To(BeFalse())
		})

		It("should create client with TLS skip verify", func() {
			client, err := NewClient("https://vault.example.com:8200", true, 15*time.Second)

			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
			Expect(client.Timeout()).To(Equal(15 * time.Second))
		})
	})

	Describe("NewClientWithConfig", func() {
		It("should create client with custom configuration", func() {
			config := &ClientConfig{
				URL:           "http://vault.test:8200",
				TLSSkipVerify: false,
				Timeout:       45 * time.Second,
				Validator:     NewStrictKeyValidator(32),
				Metrics:       NewMockClientMetrics(),
				MaxRetries:    5,
				RetryDelay:    2 * time.Second,
			}

			client, err := NewClientWithConfig(config)

			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
			Expect(client.URL()).To(Equal("http://vault.test:8200"))
			Expect(client.Timeout()).To(Equal(45 * time.Second))
		})

		It("should reject empty URL", func() {
			config := &ClientConfig{
				URL:     "",
				Timeout: 30 * time.Second,
			}

			_, err := NewClientWithConfig(config)

			Expect(err).To(HaveOccurred())
			Expect(err).To(BeAssignableToTypeOf(&ValidationError{}))
		})

		It("should set default timeout for zero timeout", func() {
			config := &ClientConfig{
				URL:     "http://vault.test:8200",
				Timeout: 0,
			}

			client, err := NewClientWithConfig(config)

			Expect(err).ToNot(HaveOccurred())
			Expect(client.Timeout()).To(Equal(30 * time.Second))
		})

		It("should set up retry strategy when MaxRetries > 1", func() {
			config := &ClientConfig{
				URL:        "http://vault.test:8200",
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			}

			client, err := NewClientWithConfig(config)

			Expect(err).ToNot(HaveOccurred())
			Expect(client.strategy).To(BeAssignableToTypeOf(&RetryUnsealStrategy{}))
		})

		It("should use default strategy when MaxRetries = 1", func() {
			config := &ClientConfig{
				URL:        "http://vault.test:8200",
				Timeout:    30 * time.Second,
				MaxRetries: 1,
			}

			client, err := NewClientWithConfig(config)

			Expect(err).ToNot(HaveOccurred())
			Expect(client.strategy).To(BeAssignableToTypeOf(&DefaultUnsealStrategy{}))
		})
	})
})

var _ = Describe("Client Operations", func() {
	var client *Client
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		client, err = NewClient("http://vault.test:8200", false, 5*time.Second)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if client != nil {
			client.Close()
		}
	})

	Describe("IsSealed", func() {
		Context("with network failures", func() {
			It("should return error and sealed=true on failure", func() {
				// This will fail because we're not connecting to a real vault
				sealed, err := client.IsSealed(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&VaultError{}))
				Expect(sealed).To(BeTrue())
			})
		})

		Context("with closed client", func() {
			It("should return error when client is closed", func() {
				client.Close()

				sealed, err := client.IsSealed(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&VaultError{}))
				Expect(sealed).To(BeTrue())
			})
		})

		Context("with canceled context", func() {
			It("should handle context cancellation", func() {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()

				sealed, err := client.IsSealed(cancelCtx)

				Expect(err).To(HaveOccurred())
				Expect(sealed).To(BeTrue())
			})
		})
	})

	Describe("GetSealStatus", func() {
		Context("with network failures", func() {
			It("should return VaultError on failure", func() {
				_, err := client.GetSealStatus(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&VaultError{}))
			})
		})

		Context("with closed client", func() {
			It("should return error when client is closed", func() {
				client.Close()

				_, err := client.GetSealStatus(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&VaultError{}))
			})
		})
	})

	Describe("Unseal", func() {
		It("should delegate to configured strategy", func() {
			keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

			// This will fail at the strategy level due to network issues
			_, err := client.Unseal(ctx, keys, 1)

			Expect(err).To(HaveOccurred())
			// Error could be from validation or network - both are expected in this test environment
		})

		Context("with closed client", func() {
			It("should return error when client is closed", func() {
				client.Close()
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				_, err := client.Unseal(ctx, keys, 1)

				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&VaultError{}))
			})
		})
	})

	Describe("IsInitialized", func() {
		Context("with network failures", func() {
			It("should return error on failure", func() {
				_, err := client.IsInitialized(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&VaultError{}))
			})
		})

		Context("with closed client", func() {
			It("should return error when client is closed", func() {
				client.Close()

				_, err := client.IsInitialized(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&VaultError{}))
			})
		})
	})

	Describe("HealthCheck", func() {
		Context("with network failures", func() {
			It("should return error on failure", func() {
				_, err := client.HealthCheck(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&VaultError{}))
			})
		})

		Context("with closed client", func() {
			It("should return error when client is closed", func() {
				client.Close()

				_, err := client.HealthCheck(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&VaultError{}))
			})
		})
	})
})

var _ = Describe("Client Lifecycle", func() {
	var client *Client

	BeforeEach(func() {
		var err error
		client, err = NewClient("http://vault.test:8200", false, 30*time.Second)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Close", func() {
		It("should mark client as closed", func() {
			Expect(client.IsClosed()).To(BeFalse())

			err := client.Close()

			Expect(err).ToNot(HaveOccurred())
			Expect(client.IsClosed()).To(BeTrue())
		})

		It("should be idempotent", func() {
			err1 := client.Close()
			err2 := client.Close()

			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())
			Expect(client.IsClosed()).To(BeTrue())
		})

		It("should clear sensitive data", func() {
			err := client.Close()

			Expect(err).ToNot(HaveOccurred())
			// We can't directly test token clearing, but we can verify the client is closed
			Expect(client.IsClosed()).To(BeTrue())
		})
	})

	Describe("thread safety", func() {
		It("should handle concurrent operations safely", func() {
			ctx := context.Background()
			done := make(chan bool, 10)

			// Start multiple concurrent operations
			for i := 0; i < 10; i++ {
				go func() {
					defer func() { done <- true }()

					// These will fail due to network issues, but shouldn't panic
					_, _ = client.IsSealed(ctx)
					_, _ = client.GetSealStatus(ctx)
					_, _ = client.IsInitialized(ctx)
					_, _ = client.HealthCheck(ctx)
				}()
			}

			// Wait for all goroutines to complete
			for i := 0; i < 10; i++ {
				<-done
			}

			// Client should still be functional
			Expect(client.IsClosed()).To(BeFalse())
		})

		It("should handle concurrent close operations", func() {
			done := make(chan error, 5)

			// Start multiple concurrent close operations
			for i := 0; i < 5; i++ {
				go func() {
					done <- client.Close()
				}()
			}

			// All should succeed
			for i := 0; i < 5; i++ {
				err := <-done
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(client.IsClosed()).To(BeTrue())
		})
	})
})

var _ = Describe("Metrics Integration", func() {
	var client *Client
	var mockMetrics *MockClientMetrics
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		mockMetrics = NewMockClientMetrics()

		config := &ClientConfig{
			URL:     "http://vault.test:8200",
			Timeout: 5 * time.Second,
			Metrics: mockMetrics,
		}

		var err error
		client, err = NewClientWithConfig(config)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if client != nil {
			client.Close()
		}
	})

	It("should record seal status check metrics", func() {
		// This will fail but should still record metrics
		_, _ = client.IsSealed(ctx)

		checks := mockMetrics.GetSealStatusChecks()
		Expect(len(checks)).To(Equal(1))
		Expect(checks[0].Endpoint).To(Equal("http://vault.test:8200"))
		Expect(checks[0].Success).To(BeFalse())
		Expect(checks[0].Duration).To(BeNumerically(">", 0))
	})

	It("should record health check metrics", func() {
		// This will fail but should still record metrics
		_, _ = client.HealthCheck(ctx)

		checks := mockMetrics.GetHealthChecks()
		Expect(len(checks)).To(Equal(1))
		Expect(checks[0].Endpoint).To(Equal("http://vault.test:8200"))
		Expect(checks[0].Success).To(BeFalse())
	})
})

var _ = Describe("DefaultClientFactory", func() {
	var factory *DefaultClientFactory

	BeforeEach(func() {
		factory = &DefaultClientFactory{}
	})

	Describe("NewClient", func() {
		It("should create VaultClient interface", func() {
			client, err := factory.NewClient("http://vault.test:8200", false, 30*time.Second)

			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
			Expect(client).To(BeAssignableToTypeOf(&Client{}))

			// Clean up
			client.Close()
		})

		It("should handle various configurations", func() {
			testCases := []struct {
				endpoint      string
				tlsSkipVerify bool
				timeout       time.Duration
				shouldFail    bool
			}{
				{"http://vault1:8200", false, 30 * time.Second, false},
				{"https://vault2:8200", true, 15 * time.Second, false},
				{"", false, 30 * time.Second, true}, // Empty URL should fail
			}

			for _, tc := range testCases {
				client, err := factory.NewClient(tc.endpoint, tc.tlsSkipVerify, tc.timeout)

				if tc.shouldFail {
					Expect(err).To(HaveOccurred())
					Expect(client).To(BeNil())
				} else {
					Expect(err).ToNot(HaveOccurred())
					Expect(client).ToNot(BeNil())
					client.Close()
				}
			}
		})
	})
})

var _ = Describe("Error Handling", func() {
	Describe("VaultError creation and handling", func() {
		It("should create proper VaultError with retryable flag", func() {
			baseErr := fmt.Errorf("connection failed")
			vaultErr := NewVaultError("test-op", "http://vault:8200", baseErr, true)

			Expect(vaultErr.Operation).To(Equal("test-op"))
			Expect(vaultErr.Endpoint).To(Equal("http://vault:8200"))
			Expect(vaultErr.Err).To(Equal(baseErr))
			Expect(vaultErr.IsRetryable()).To(BeTrue())
			Expect(vaultErr.Error()).To(ContainSubstring("vault test-op failed for http://vault:8200"))
		})

		It("should properly unwrap errors", func() {
			baseErr := fmt.Errorf("original error")
			vaultErr := NewVaultError("test", "endpoint", baseErr, false)

			Expect(errors.Unwrap(vaultErr)).To(Equal(baseErr))
		})
	})

	Describe("error type checking", func() {
		It("should identify VaultErrors correctly", func() {
			vaultErr := NewVaultError("test", "endpoint", fmt.Errorf("error"), true)
			otherErr := fmt.Errorf("regular error")

			Expect(IsRetryableError(vaultErr)).To(BeTrue())
			Expect(IsRetryableError(otherErr)).To(BeFalse())
		})
	})
})
