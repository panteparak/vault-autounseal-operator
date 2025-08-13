package vault

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client Configuration Validation", func() {
	Describe("URL Validation", func() {
		Context("when URL is invalid", func() {
			It("should reject empty URL", func() {
				config := &ClientConfig{
					URL:     "",
					Timeout: 30 * time.Second,
				}

				client, err := NewClientWithConfig(config)
				Expect(err).To(HaveOccurred())
				Expect(client).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("URL cannot be empty"))
				Expect(IsValidationError(err)).To(BeTrue())
			})

			It("should reject invalid URL schemes", func() {
				invalidURLs := []string{
					"not-a-url",
					"ftp://vault.example.com",
					"file:///etc/passwd",
					"javascript:alert('xss')",
				}

				for _, url := range invalidURLs {
					config := &ClientConfig{
						URL:     url,
						Timeout: 30 * time.Second,
					}

					client, err := NewClientWithConfig(config)
					Expect(err).To(HaveOccurred(), "URL should be rejected: %s", url)
					Expect(client).To(BeNil())
					Expect(err.Error()).To(ContainSubstring("must start with http"))
				}
			})

			It("should reject extremely long URLs to prevent DoS", func() {
				longURL := "https://" + strings.Repeat("very-long-hostname", 200) + ":8200"
				config := &ClientConfig{
					URL:     longURL,
					Timeout: 30 * time.Second,
				}

				client, err := NewClientWithConfig(config)
				Expect(err).To(HaveOccurred())
				Expect(client).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("exceeds maximum length"))
			})
		})

		Context("when URL is valid", func() {
			It("should accept HTTPS URLs", func() {
				config := &ClientConfig{
					URL:     "https://vault.example.com:8200",
					Timeout: 30 * time.Second,
				}

				client, err := NewClientWithConfig(config)
				Expect(err).ToNot(HaveOccurred())
				Expect(client).ToNot(BeNil())
				defer client.Close()
			})

			It("should accept HTTP URLs", func() {
				config := &ClientConfig{
					URL:     "http://vault.example.com:8200",
					Timeout: 30 * time.Second,
				}

				client, err := NewClientWithConfig(config)
				Expect(err).ToNot(HaveOccurred())
				Expect(client).ToNot(BeNil())
				defer client.Close()
			})
		})
	})

	Describe("Timeout Configuration", func() {
		Context("when timeout is invalid", func() {
			It("should reject extremely small timeouts", func() {
				config := &ClientConfig{
					URL:     "https://vault.example.com:8200",
					Timeout: 500 * time.Nanosecond,
				}

				client, err := NewClientWithConfig(config)
				Expect(err).To(HaveOccurred())
				Expect(client).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("at least 1 millisecond"))
			})
		})

		Context("when timeout is valid", func() {
			It("should accept reasonable timeout values", func() {
				validTimeouts := []time.Duration{
					1 * time.Millisecond,
					1 * time.Second,
					30 * time.Second,
					5 * time.Minute,
				}

				for _, timeout := range validTimeouts {
					config := &ClientConfig{
						URL:     "https://vault.example.com:8200",
						Timeout: timeout,
					}

					client, err := NewClientWithConfig(config)
					Expect(err).ToNot(HaveOccurred(), "Timeout should be accepted: %v", timeout)
					Expect(client).ToNot(BeNil())
					client.Close()
				}
			})
		})
	})

	Describe("Retry Configuration", func() {
		Context("when retry settings are invalid", func() {
			It("should reject negative retry values", func() {
				config := &ClientConfig{
					URL:        "https://vault.example.com:8200",
					Timeout:    30 * time.Second,
					MaxRetries: -5,
				}

				client, err := NewClientWithConfig(config)
				Expect(err).To(HaveOccurred())
				Expect(client).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("cannot be negative"))
			})
		})

		Context("when retry settings are valid", func() {
			It("should accept zero retries (no retry)", func() {
				config := &ClientConfig{
					URL:        "https://vault.example.com:8200",
					Timeout:    30 * time.Second,
					MaxRetries: 0,
				}

				client, err := NewClientWithConfig(config)
				Expect(err).ToNot(HaveOccurred())
				Expect(client).ToNot(BeNil())
				defer client.Close()
			})

			It("should accept reasonable retry counts", func() {
				validRetries := []int{1, 3, 5, 10}

				for _, retries := range validRetries {
					config := &ClientConfig{
						URL:        "https://vault.example.com:8200",
						Timeout:    30 * time.Second,
						MaxRetries: retries,
					}

					client, err := NewClientWithConfig(config)
					Expect(err).ToNot(HaveOccurred(), "Retries should be accepted: %d", retries)
					Expect(client).ToNot(BeNil())
					client.Close()
				}
			})
		})
	})

	Describe("Complete Configuration Validation", func() {
		It("should validate all parameters together", func() {
			config := &ClientConfig{
				URL:        "https://vault.prod.example.com:8200",
				Timeout:    45 * time.Second,
				MaxRetries: 5,
			}

			client, err := NewClientWithConfig(config)
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
			defer client.Close()

			// Verify client is not closed initially
			Expect(client.IsClosed()).To(BeFalse())
		})

		It("should provide meaningful error messages", func() {
			// Test each validation error contains helpful context
			testCases := []struct {
				config         *ClientConfig
				expectedSubstr string
				description    string
			}{
				{
					config:         &ClientConfig{URL: "", Timeout: 30 * time.Second},
					expectedSubstr: "URL cannot be empty",
					description:    "empty URL error",
				},
				{
					config:         &ClientConfig{URL: "https://valid.com", Timeout: 500 * time.Nanosecond},
					expectedSubstr: "at least 1 millisecond",
					description:    "sub-millisecond timeout error",
				},
				{
					config:         &ClientConfig{URL: "https://valid.com", Timeout: 30 * time.Second, MaxRetries: -1},
					expectedSubstr: "cannot be negative",
					description:    "negative retries error",
				},
			}

			for _, tc := range testCases {
				client, err := NewClientWithConfig(tc.config)
				Expect(err).To(HaveOccurred(), "Should fail for %s", tc.description)
				Expect(client).To(BeNil())
				Expect(err.Error()).To(ContainSubstring(tc.expectedSubstr))
				Expect(IsValidationError(err)).To(BeTrue())
			}
		})
	})
})
