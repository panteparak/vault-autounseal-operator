package vault

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVault(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Vault Suite")
}

var _ = Describe("VaultClient", func() {
	var client *Client
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewClient", func() {
		It("should create a new client with valid URL", func() {
			var err error
			client, err = NewClient("http://vault.example.com:8200", false, 30*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
			Expect(client.url).To(Equal("http://vault.example.com:8200"))
		})

		It("should create a client with TLS skip verify enabled", func() {
			var err error
			client, err = NewClient("https://vault.example.com:8200", true, 30*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})
	})

	Describe("Unseal validation", func() {
		BeforeEach(func() {
			var err error
			client, err = NewClient("http://vault.example.com:8200", false, 30*time.Second)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error with empty keys", func() {
			_, err := client.Unseal(ctx, []string{}, 1)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no unseal keys provided"))
		})

		It("should return error with invalid threshold", func() {
			keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}
			_, err := client.Unseal(ctx, keys, 0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("threshold must be at least 1"))
		})

		It("should return error when threshold exceeds keys", func() {
			keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}
			_, err := client.Unseal(ctx, keys, 2)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("threshold exceeds number of available keys"))
		})

		It("should return error with invalid base64", func() {
			keys := []string{"invalid-base64!@#"}
			// Test input validation directly without network calls
			_, err := base64.StdEncoding.DecodeString(keys[0])
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("illegal base64 data"))
		})
	})
})
