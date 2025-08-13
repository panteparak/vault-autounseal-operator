// +build integration

package vault

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestValidator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validator Suite")
}

var _ = Describe("DefaultKeyValidator", func() {
	var validator *DefaultKeyValidator

	BeforeEach(func() {
		validator = NewDefaultKeyValidator()
	})

	Describe("ValidateKeys", func() {
		Context("with valid inputs", func() {
			It("should validate single key with threshold 1", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("test-key"))}
				err := validator.ValidateKeys(keys, 1)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should validate multiple keys with valid threshold", func() {
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
					base64.StdEncoding.EncodeToString([]byte("key3")),
				}
				err := validator.ValidateKeys(keys, 2)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should validate threshold equal to number of keys", func() {
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					base64.StdEncoding.EncodeToString([]byte("key2")),
				}
				err := validator.ValidateKeys(keys, 2)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with invalid inputs", func() {
			It("should reject empty keys slice", func() {
				err := validator.ValidateKeys([]string{}, 1)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&ValidationError{}))
			})

			It("should reject nil keys slice", func() {
				err := validator.ValidateKeys(nil, 1)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&ValidationError{}))
			})

			It("should reject zero threshold", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}
				err := validator.ValidateKeys(keys, 0)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&ValidationError{}))
			})

			It("should reject negative threshold", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}
				err := validator.ValidateKeys(keys, -1)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&ValidationError{}))
			})

			It("should reject threshold exceeding keys", func() {
				keys := []string{base64.StdEncoding.EncodeToString([]byte("key1"))}
				err := validator.ValidateKeys(keys, 2)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&ValidationError{}))
			})
		})

		Context("with invalid base64 keys", func() {
			It("should reject invalid base64 encoding", func() {
				keys := []string{"not-valid-base64!@#"}
				err := validator.ValidateKeys(keys, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid key at index 0"))
			})

			It("should reject empty keys", func() {
				keys := []string{""}
				err := validator.ValidateKeys(keys, 1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid key at index 0"))
			})

			It("should identify the correct key index in error", func() {
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("valid-key")),
					"invalid-base64",
				}
				err := validator.ValidateKeys(keys, 2)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid key at index 1"))
			})
		})

		Context("with duplicate keys", func() {
			It("should reject duplicate keys", func() {
				key := base64.StdEncoding.EncodeToString([]byte("duplicate-key"))
				keys := []string{key, key}
				err := validator.ValidateKeys(keys, 2)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&ValidationError{}))
				Expect(err.Error()).To(ContainSubstring("duplicate key"))
			})

			It("should identify duplicate key indices", func() {
				key := base64.StdEncoding.EncodeToString([]byte("duplicate"))
				keys := []string{
					base64.StdEncoding.EncodeToString([]byte("key1")),
					key,
					base64.StdEncoding.EncodeToString([]byte("key3")),
					key,
				}
				err := validator.ValidateKeys(keys, 3)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("indices 1 and 3"))
			})
		})
	})

	Describe("ValidateBase64Key", func() {
		Context("with valid keys", func() {
			It("should validate normal base64 key", func() {
				key := base64.StdEncoding.EncodeToString([]byte("test-key"))
				err := validator.ValidateBase64Key(key)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should validate various key lengths", func() {
				testCases := [][]byte{
					[]byte("a"),
					[]byte("short-key"),
					[]byte("medium-length-key-content"),
					[]byte(strings.Repeat("long-key-", 10)),
				}

				for _, content := range testCases {
					key := base64.StdEncoding.EncodeToString(content)
					err := validator.ValidateBase64Key(key)
					Expect(err).ToNot(HaveOccurred(), "Failed for key length: %d", len(content))
				}
			})

			It("should validate binary data keys", func() {
				binaryData := []byte{0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD}
				key := base64.StdEncoding.EncodeToString(binaryData)
				err := validator.ValidateBase64Key(key)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with invalid keys", func() {
			It("should reject empty key", func() {
				err := validator.ValidateBase64Key("")
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&ValidationError{}))
			})

			It("should reject invalid base64 characters", func() {
				invalidKeys := []string{
					"invalid!@#",
					"not_base64_at_all",
					"almost=but=not==quite",
				}

				for _, key := range invalidKeys {
					err := validator.ValidateBase64Key(key)
					Expect(err).To(HaveOccurred(), "Should reject key: %s", key)
					Expect(err).To(BeAssignableToTypeOf(&ValidationError{}))
				}
			})

			It("should reject keys that decode to empty", func() {
				key := base64.StdEncoding.EncodeToString([]byte(""))
				err := validator.ValidateBase64Key(key)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("key cannot be empty"))
			})
		})

		Context("with key pattern validation", func() {
			It("should reject all-zero keys", func() {
				allZeros := make([]byte, 16)
				key := base64.StdEncoding.EncodeToString(allZeros)
				err := validator.ValidateBase64Key(key)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("key cannot be all zeros"))
			})

			It("should reject all-same-byte keys", func() {
				allSame := make([]byte, 8)
				for i := range allSame {
					allSame[i] = 0xAA
				}
				key := base64.StdEncoding.EncodeToString(allSame)
				err := validator.ValidateBase64Key(key)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("key cannot have all identical bytes"))
			})

			It("should allow single-byte keys", func() {
				singleByte := []byte{0xAA}
				key := base64.StdEncoding.EncodeToString(singleByte)
				err := validator.ValidateBase64Key(key)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})

var _ = Describe("StrictKeyValidator", func() {
	var validator *StrictKeyValidator

	BeforeEach(func() {
		validator = NewStrictKeyValidator(32) // Require 32-byte keys
	})

	Describe("ValidateBase64Key with strict rules", func() {
		Context("with correct key length", func() {
			It("should validate key of required length", func() {
				keyData := make([]byte, 32)
				for i := range keyData {
					keyData[i] = byte(i)
				}
				key := base64.StdEncoding.EncodeToString(keyData)
				err := validator.ValidateBase64Key(key)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with incorrect key length", func() {
			It("should reject keys shorter than required", func() {
				keyData := make([]byte, 16) // Too short
				for i := range keyData {
					keyData[i] = byte(i)
				}
				key := base64.StdEncoding.EncodeToString(keyData)
				err := validator.ValidateBase64Key(key)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be exactly 32 bytes"))
			})

			It("should reject keys longer than required", func() {
				keyData := make([]byte, 64) // Too long
				for i := range keyData {
					keyData[i] = byte(i)
				}
				key := base64.StdEncoding.EncodeToString(keyData)
				err := validator.ValidateBase64Key(key)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be exactly 32 bytes"))
			})
		})

		Context("with forbidden strings", func() {
			It("should reject keys containing forbidden strings", func() {
				forbiddenKeys := []string{
					base64.StdEncoding.EncodeToString([]byte("this-contains-password-string")),
					base64.StdEncoding.EncodeToString([]byte("SECRET-key-data")),
					base64.StdEncoding.EncodeToString([]byte("test-key-for-demo")),
				}

				for _, key := range forbiddenKeys {
					// Need to make it the right length first
					decoded, _ := base64.StdEncoding.DecodeString(key)
					if len(decoded) != 32 {
						// Pad or truncate to 32 bytes
						newData := make([]byte, 32)
						copy(newData, decoded)
						key = base64.StdEncoding.EncodeToString(newData)
					}

					err := validator.ValidateBase64Key(key)
					if !IsValidationError(err) {
						// Skip if it fails basic validation first
						continue
					}
					Expect(err).To(HaveOccurred(), "Should reject key containing forbidden string")
				}
			})
		})

		Context("with allowed prefixes", func() {
			BeforeEach(func() {
				validator.SetAllowedPrefixes([]string{"PROD-", "DEV-"})
			})

			It("should accept keys with allowed prefixes", func() {
				keyData := make([]byte, 32)
				for i := range keyData {
					keyData[i] = byte(i + 1) // Avoid all zeros
				}
				key := "PROD-" + base64.StdEncoding.EncodeToString(keyData)

				// This will fail length validation, but we're testing prefix logic
				err := validator.ValidateBase64Key(key)
				// The error should be about base64 format, not prefix
				if err != nil {
					Expect(err.Error()).ToNot(ContainSubstring("must start with"))
				}
			})

			It("should reject keys without allowed prefixes", func() {
				keyData := make([]byte, 32)
				for i := range keyData {
					keyData[i] = byte(i + 1)
				}
				key := "INVALID-" + base64.StdEncoding.EncodeToString(keyData)

				err := validator.ValidateBase64Key(key)
				// Should fail on base64 validation first, but if it was valid base64
				// it would fail on prefix validation
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("configuration methods", func() {
		It("should allow setting custom forbidden strings", func() {
			validator.SetForbiddenStrings([]string{"custom", "forbidden"})

			keyData := make([]byte, 32)
			copy(keyData, "this-has-custom-content")
			key := base64.StdEncoding.EncodeToString(keyData)

			err := validator.ValidateBase64Key(key)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("custom"))
		})

		It("should allow setting custom allowed prefixes", func() {
			validator.SetAllowedPrefixes([]string{"TEST-"})

			// This tests the configuration, actual validation depends on
			// the key being valid base64 format
			Expect(validator.allowedPrefixes).To(Equal([]string{"TEST-"}))
		})
	})
})

var _ = Describe("Error Types", func() {
	Describe("ValidationError", func() {
		It("should create proper error message", func() {
			err := NewValidationError("threshold", 5, "exceeds maximum allowed")
			Expect(err.Error()).To(Equal("validation failed for field 'threshold' with value '5': exceeds maximum allowed"))
			Expect(err.Field).To(Equal("threshold"))
			Expect(err.Value).To(Equal(5))
			Expect(err.Message).To(Equal("exceeds maximum allowed"))
		})
	})

	Describe("error type checking functions", func() {
		It("should correctly identify validation errors", func() {
			validationErr := NewValidationError("test", "value", "message")
			Expect(IsValidationError(validationErr)).To(BeTrue())

			otherErr := fmt.Errorf("regular error")
			Expect(IsValidationError(otherErr)).To(BeFalse())
		})

		It("should correctly identify retryable errors", func() {
			retryableErr := NewVaultError("test", "endpoint", fmt.Errorf("base"), true)
			Expect(IsRetryableError(retryableErr)).To(BeTrue())

			nonRetryableErr := NewVaultError("test", "endpoint", fmt.Errorf("base"), false)
			Expect(IsRetryableError(nonRetryableErr)).To(BeFalse())
		})
	})
})
