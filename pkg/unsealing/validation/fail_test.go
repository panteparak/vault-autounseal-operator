package validation

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidationFailures tests various validation failure scenarios
func TestValidationFailures(t *testing.T) {
	validator := NewDefaultKeyValidator()

	t.Run("should fail with empty keys", func(t *testing.T) {
		err := validator.ValidateKeys([]string{}, 1)
		assert.Error(t, err, "Empty keys should fail validation")
		assert.Contains(t, err.Error(), "no unseal keys provided")
	})

	t.Run("should fail with nil threshold", func(t *testing.T) {
		validKey := base64.StdEncoding.EncodeToString([]byte("test-key"))
		err := validator.ValidateKeys([]string{validKey}, 0)
		assert.Error(t, err, "Zero threshold should fail validation")
		assert.Contains(t, err.Error(), "threshold must be at least 1")
	})

	t.Run("should fail with threshold exceeding keys", func(t *testing.T) {
		validKey := base64.StdEncoding.EncodeToString([]byte("test-key"))
		err := validator.ValidateKeys([]string{validKey}, 5)
		assert.Error(t, err, "Threshold exceeding keys should fail")
		assert.Contains(t, err.Error(), "threshold (5) exceeds number of available keys (1)")
	})

	t.Run("should fail with invalid base64", func(t *testing.T) {
		err := validator.ValidateBase64Key("not-valid-base64!@#$%")
		assert.Error(t, err, "Invalid base64 should fail validation")
		assert.Contains(t, err.Error(), "invalid base64 encoding")
	})

	t.Run("should fail with all-zero key", func(t *testing.T) {
		zeroKey := base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 0, 0})
		err := validator.ValidateBase64Key(zeroKey)
		assert.Error(t, err, "All-zero key should fail validation")
		assert.Contains(t, err.Error(), "key cannot be all zeros")
	})

	t.Run("should fail with repeating pattern key", func(t *testing.T) {
		// Create a key with obvious repeating pattern
		repeatingKey := base64.StdEncoding.EncodeToString([]byte{1, 2, 1, 2, 1, 2, 1, 2})
		err := validator.ValidateBase64Key(repeatingKey)
		assert.Error(t, err, "Repeating pattern key should fail validation")
		assert.Contains(t, err.Error(), "weak repeating pattern")
	})

	t.Run("should fail with duplicate keys", func(t *testing.T) {
		duplicateKey := base64.StdEncoding.EncodeToString([]byte("duplicate-key"))
		err := validator.ValidateKeys([]string{duplicateKey, duplicateKey}, 2)
		assert.Error(t, err, "Duplicate keys should fail validation")
		assert.Contains(t, err.Error(), "duplicate key found")
	})
}

// TestStrictValidationFailures tests strict validator failure scenarios
func TestStrictValidationFailures(t *testing.T) {
	validator := NewStrictKeyValidator(32) // Require 32-byte keys
	validator.SetAllowedPrefixes([]string{"vault-", "prod-"})
	validator.SetForbiddenStrings([]string{"test", "demo", "admin"})

	t.Run("should fail with wrong key length", func(t *testing.T) {
		shortKey := base64.StdEncoding.EncodeToString([]byte("short"))
		err := validator.ValidateBase64Key(shortKey)
		assert.Error(t, err, "Wrong length key should fail strict validation")
		assert.Contains(t, err.Error(), "key must be exactly 32 bytes")
	})

	t.Run("should fail with invalid prefix", func(t *testing.T) {
		invalidPrefixKey := base64.StdEncoding.EncodeToString([]byte("dev-some-key-that-is-exactly-32b"))
		err := validator.ValidateBase64Key(invalidPrefixKey)
		assert.Error(t, err, "Invalid prefix should fail strict validation")
		assert.Contains(t, err.Error(), "decoded key must start with one of")
	})

	t.Run("should fail with forbidden string", func(t *testing.T) {
		// Create exactly 32 bytes with forbidden content
		forbiddenKey := base64.StdEncoding.EncodeToString([]byte("vault-admin-key-exactly-32-bytes"))
		err := validator.ValidateBase64Key(forbiddenKey)
		assert.Error(t, err, "Forbidden string should fail strict validation")
		assert.Contains(t, err.Error(), "key contains forbidden string")
	})
}

// TestSensitiveContentRedaction tests that sensitive content is properly redacted
func TestSensitiveContentRedaction(t *testing.T) {
	validator := NewDefaultKeyValidator()

	t.Run("should redact sensitive content in error messages", func(t *testing.T) {
		// Try to validate a key that contains sensitive content
		sensitiveKey := "password123"
		err := validator.ValidateBase64Key(sensitiveKey)

		assert.Error(t, err, "Sensitive key should fail validation")
		assert.Contains(t, err.Error(), "[REDACTED]", "Error should contain redacted placeholder")
		assert.NotContains(t, err.Error(), "password123", "Error should not contain actual sensitive content")
	})

	t.Run("should redact localhost patterns", func(t *testing.T) {
		// Try to validate a key that contains localhost
		localhostKey := "localhost:8200"
		err := validator.ValidateBase64Key(localhostKey)

		assert.Error(t, err, "Localhost key should fail validation")
		assert.Contains(t, err.Error(), "[REDACTED]", "Error should contain redacted placeholder")
		assert.NotContains(t, err.Error(), "localhost", "Error should not contain actual localhost")
	})
}

// TestEdgeCaseFailures tests edge case failure scenarios
func TestEdgeCaseFailures(t *testing.T) {
	validator := NewDefaultKeyValidator()

	t.Run("should fail with empty decoded key", func(t *testing.T) {
		// Base64 encoding of empty string
		emptyKey := base64.StdEncoding.EncodeToString([]byte{})
		err := validator.ValidateBase64Key(emptyKey)
		assert.Error(t, err, "Empty decoded key should fail validation")
		assert.Contains(t, err.Error(), "key cannot be empty")
	})

	t.Run("should fail with all identical bytes", func(t *testing.T) {
		// Key with all identical bytes
		identicalKey := base64.StdEncoding.EncodeToString([]byte{65, 65, 65, 65, 65})
		err := validator.ValidateBase64Key(identicalKey)
		assert.Error(t, err, "All identical bytes should fail validation")
		assert.Contains(t, err.Error(), "key cannot have all identical bytes")
	})

	t.Run("should fail with extremely long keys", func(t *testing.T) {
		// Create a very long key (over the limit)
		longContent := make([]byte, 2000) // Much longer than maxKeyLength
		for i := range longContent {
			longContent[i] = byte(i % 256)
		}
		longKey := base64.StdEncoding.EncodeToString(longContent)

		err := validator.ValidateBase64Key(longKey)
		assert.Error(t, err, "Extremely long key should fail validation")
		assert.Contains(t, err.Error(), "exceeds maximum")
	})
}

// TestConcurrentValidationFailures tests validation failures under concurrent access
func TestConcurrentValidationFailures(t *testing.T) {
	validator := NewDefaultKeyValidator()

	t.Run("should handle concurrent validation failures", func(t *testing.T) {
		// Run multiple concurrent validations that should all fail
		errorChan := make(chan error, 10)

		for i := 0; i < 10; i++ {
			go func(index int) {
				invalidKey := "invalid-key-" + string(rune(index))
				err := validator.ValidateBase64Key(invalidKey)
				errorChan <- err
			}(i)
		}

		// Collect all errors
		for i := 0; i < 10; i++ {
			err := <-errorChan
			assert.Error(t, err, "All concurrent validations should fail")
		}
	})
}
