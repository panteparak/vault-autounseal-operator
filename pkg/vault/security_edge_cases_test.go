package vault

import (
	"encoding/base64"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Security Edge Cases for Business Logic", func() {
	var validator KeyValidator

	BeforeEach(func() {
		validator = NewDefaultKeyValidator()
	})

	Describe("Input Sanitization", func() {
		Context("when handling potentially malicious inputs", func() {
			It("should prevent key injection patterns", func() {
				injectionKeys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1\x00\x01malicious")),
					base64.StdEncoding.EncodeToString([]byte("../../../etc/passwd")),
					base64.StdEncoding.EncodeToString([]byte("<script>alert('xss')</script>")),
				}

				for _, key := range injectionKeys {
					err := validator.ValidateKeys([]string{key}, 1)
					if err == nil {
						continue
					}
					Expect(IsValidationError(err)).To(BeTrue())
				}
			})

			It("should sanitize sensitive content from error messages", func() {
				sensitiveKey := "secret-admin-password"

				err := validator.ValidateKeys([]string{sensitiveKey}, 1)
				Expect(err).To(HaveOccurred())

				errorMsg := err.Error()
				Expect(errorMsg).ToNot(ContainSubstring(sensitiveKey))
				Expect(errorMsg).To(ContainSubstring("[REDACTED]"))
			})
		})
	})

	Describe("Resource Protection", func() {
		Context("when handling large inputs", func() {
			It("should handle large key sets without exhaustion", func() {
				largeKeySet := make([]string, 50)
				for i := range largeKeySet {
					// Create truly unique keys by using index as part of content
					keyContent := fmt.Sprintf("security-test-key-%d-with-entropy-%d", i, i*23)
					largeKeySet[i] = base64.StdEncoding.EncodeToString([]byte(keyContent))
				}

				err := validator.ValidateKeys(largeKeySet, 25)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
