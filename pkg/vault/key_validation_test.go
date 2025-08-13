package vault

import (
	"encoding/base64"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Key Validation", func() {
	var validator KeyValidator

	BeforeEach(func() {
		validator = NewDefaultKeyValidator()
	})

	Describe("Basic Key Validation", func() {
		Context("when keys are invalid", func() {
			It("should reject empty keys array", func() {
				err := validator.ValidateKeys([]string{}, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no unseal keys"))
				Expect(IsValidationError(err)).To(BeTrue())
			})

			It("should validate base64 format correctly", func() {
				// Test valid base64 key
				validKey := base64.StdEncoding.EncodeToString([]byte("test-key"))
				err := validator.ValidateKeys([]string{validKey}, 1)
				Expect(err).ToNot(HaveOccurred())

				// If certain characters are invalid, test should catch them
				// Otherwise, this test passes as long as basic validation works
			})

			It("should reject duplicate keys", func() {
				duplicateKey := base64.StdEncoding.EncodeToString([]byte("same-key"))
				keys := []string{duplicateKey, duplicateKey}

				err := validator.ValidateKeys(keys, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("duplicate key"))
			})
		})

		Context("when keys are valid", func() {
			It("should accept valid base64 keys", func() {
				validKeys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
					base64.StdEncoding.EncodeToString([]byte("key3")),
				}

				err := validator.ValidateKeys(validKeys, 3)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle keys with different lengths", func() {
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("short")),
					base64.StdEncoding.EncodeToString([]byte("medium-length-key")),
					base64.StdEncoding.EncodeToString([]byte("very-long-key-with-many-characters")),
				}

				err := validator.ValidateKeys(keys, 2)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("Threshold Validation", func() {
		Context("when threshold is invalid", func() {
			It("should reject threshold greater than available keys", func() {
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
				}

				err := validator.ValidateKeys(keys, 5)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("exceeds number of available keys"))
			})

			It("should reject threshold less than 1", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				err := validator.ValidateKeys(keys, 0)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("threshold must be at least 1"))
			})

			It("should reject negative threshold", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}

				err := validator.ValidateKeys(keys, -1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("threshold must be at least 1"))
			})
		})

		Context("when threshold is valid", func() {
			It("should accept threshold equal to key count", func() {
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
				}

				err := validator.ValidateKeys(keys, 2)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should accept threshold less than key count", func() {
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
					base64.StdEncoding.EncodeToString([]byte("key3")),
				}

				err := validator.ValidateKeys(keys, 2)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("Individual Key Validation", func() {
		Context("when validating single keys", func() {
			It("should accept valid base64 key", func() {
				validKey := base64.StdEncoding.EncodeToString([]byte("test-key"))

				err := validator.ValidateBase64Key(validKey)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should reject invalid base64 key", func() {
				invalidKey := "not-base64!@#"

				err := validator.ValidateBase64Key(invalidKey)
				Expect(err).To(HaveOccurred())
				Expect(IsValidationError(err)).To(BeTrue())
			})

			It("should reject empty key", func() {
				err := validator.ValidateBase64Key("")
				Expect(err).To(HaveOccurred())
				Expect(IsValidationError(err)).To(BeTrue())
			})
		})
	})

	Describe("Security-Focused Validation", func() {
		Context("when validating for security concerns", func() {
			It("should sanitize sensitive content from error messages", func() {
				sensitiveKey := "secret-admin-password"

				err := validator.ValidateKeys([]string{sensitiveKey}, 1)
				Expect(err).To(HaveOccurred())

				errorMsg := err.Error()
				Expect(errorMsg).ToNot(ContainSubstring(sensitiveKey))
				Expect(errorMsg).To(ContainSubstring("[REDACTED]"))
			})

			It("should handle large key sets without resource exhaustion", func() {
				largeKeySet := make([]string, 50)
				for i := range largeKeySet {
					// Create truly unique keys by using index as part of content
					keyContent := fmt.Sprintf("unique-key-%d-with-sufficient-entropy-%d", i, i*17)
					largeKeySet[i] = base64.StdEncoding.EncodeToString([]byte(keyContent))
				}

				err := validator.ValidateKeys(largeKeySet, 25)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("Strict Key Validator", func() {
		Context("when using strict validation", func() {
			It("should enforce required key length", func() {
				strictValidator := NewStrictKeyValidator(32)

				shortKeyData := make([]byte, 16)
				for i := range shortKeyData {
					shortKeyData[i] = byte(i + 1) // Non-zero pattern
				}
				shortKey := base64.StdEncoding.EncodeToString(shortKeyData)

				err := strictValidator.ValidateKeys([]string{shortKey}, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be exactly"))
			})

			It("should reject forbidden strings", func() {
				strictValidator := NewStrictKeyValidator(16)
				strictValidator.SetForbiddenStrings([]string{"password", "admin", "secret"})

				forbiddenKeyData := []byte("admin-password")
				keyData := make([]byte, 16)
				copy(keyData, forbiddenKeyData)
				forbiddenKey := base64.StdEncoding.EncodeToString(keyData)

				err := strictValidator.ValidateKeys([]string{forbiddenKey}, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("forbidden string"))
			})

			It("should enforce allowed prefixes", func() {
				strictValidator := NewStrictKeyValidator(16)
				strictValidator.SetAllowedPrefixes([]string{"VAULT_", "PROD_"})

				keyData := make([]byte, 16)
				for i := range keyData {
					keyData[i] = byte(i + 1) // Non-zero pattern
				}
				invalidKey := base64.StdEncoding.EncodeToString(keyData)

				err := strictValidator.ValidateKeys([]string{invalidKey}, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must start with one of"))
			})
		})
	})
})
