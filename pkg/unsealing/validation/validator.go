package validation

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/panteparak/vault-autounseal-operator/pkg/core/types"
)

// DefaultKeyValidator implements the KeyValidator interface
type DefaultKeyValidator struct {
	minKeyLength int
	maxKeyLength int
}

// NewDefaultKeyValidator creates a new key validator with default settings
func NewDefaultKeyValidator() *DefaultKeyValidator {
	return &DefaultKeyValidator{
		minKeyLength: 1,
		maxKeyLength: 1024, // Reasonable max for base64 encoded keys
	}
}

// ValidateKeys validates a set of unseal keys and threshold
func (v *DefaultKeyValidator) ValidateKeys(keys []string, threshold int) error {
	if len(keys) == 0 {
		return types.NewValidationError("keys", keys, "no unseal keys provided")
	}

	if threshold < 1 {
		return types.NewValidationError("threshold", threshold, "threshold must be at least 1")
	}

	if threshold > len(keys) {
		return types.NewValidationError("threshold", threshold,
			fmt.Sprintf("threshold (%d) exceeds number of available keys (%d)", threshold, len(keys)))
	}

	// Validate each key
	for i, key := range keys {
		if err := v.ValidateBase64Key(key); err != nil {
			return fmt.Errorf("invalid key at index %d: %w", i, err)
		}
	}

	// Check for duplicate keys
	if err := v.checkForDuplicates(keys); err != nil {
		return err
	}

	return nil
}

// ValidateBase64Key validates a single base64 encoded key
func (v *DefaultKeyValidator) ValidateBase64Key(key string) error {
	// Check for sensitive content early and redact if necessary
	displayKey := key
	if v.containsSensitiveContent(key) {
		displayKey = "[REDACTED]"
	}

	if key == "" {
		return types.NewValidationError("key", displayKey, "key cannot be empty")
	}

	if len(key) < v.minKeyLength {
		return types.NewValidationError("key", displayKey,
			fmt.Sprintf("key length (%d) is below minimum (%d)", len(key), v.minKeyLength))
	}

	if len(key) > v.maxKeyLength {
		return types.NewValidationError("key", displayKey,
			fmt.Sprintf("key length (%d) exceeds maximum (%d)", len(key), v.maxKeyLength))
	}

	// Validate base64 encoding
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return types.NewValidationError("key", displayKey, fmt.Sprintf("invalid base64 encoding: %v", err))
	}

	// Check decoded key length
	if len(decoded) == 0 {
		return types.NewValidationError("key", displayKey, "decoded key cannot be empty")
	}

	// Additional validation for key patterns
	if err := v.validateKeyPattern(decoded); err != nil {
		// The validateKeyPattern error might also contain sensitive data
		if v.containsSensitiveContent(key) {
			return types.NewValidationError("key", displayKey, "key validation failed")
		}
		return err
	}

	return nil
}

// checkForDuplicates checks if there are duplicate keys in the slice
func (v *DefaultKeyValidator) checkForDuplicates(keys []string) error {
	seen := make(map[string]int)
	for i, key := range keys {
		if prevIndex, exists := seen[key]; exists {
			return types.NewValidationError("keys", keys,
				fmt.Sprintf("duplicate key found at indices %d and %d", prevIndex, i))
		}
		seen[key] = i
	}
	return nil
}

// validateKeyPattern performs additional validation on decoded key data
func (v *DefaultKeyValidator) validateKeyPattern(decoded []byte) error {
	// Check for obviously invalid patterns
	allZeros := true
	allSame := true
	firstByte := decoded[0]

	for _, b := range decoded {
		if b != 0 {
			allZeros = false
		}
		if b != firstByte {
			allSame = false
		}
	}

	if allZeros {
		return types.NewValidationError("key", decoded, "key cannot be all zeros")
	}

	if allSame && len(decoded) > 1 {
		return types.NewValidationError("key", decoded, "key cannot have all identical bytes")
	}

	// Check for simple repeating patterns
	if len(decoded) >= 8 {
		// Check for 2-byte patterns
		if hasRepeatingPattern(decoded, 2) {
			return types.NewValidationError("key", decoded, "key contains weak repeating pattern")
		}
		// Check for 4-byte patterns
		if hasRepeatingPattern(decoded, 4) {
			return types.NewValidationError("key", decoded, "key contains weak repeating pattern")
		}
	}

	return nil
}

// hasRepeatingPattern checks if the byte slice contains a repeating pattern of the given length
func hasRepeatingPattern(data []byte, patternLen int) bool {
	if len(data) < patternLen*2 {
		return false
	}

	// Extract the pattern from the beginning
	pattern := data[:patternLen]

	// Check if the pattern repeats throughout the data
	for i := patternLen; i <= len(data)-patternLen; i += patternLen {
		if !bytes.Equal(pattern, data[i:i+patternLen]) {
			return false
		}
	}

	// Check if the remaining bytes (if any) match the beginning of the pattern
	remaining := len(data) % patternLen
	if remaining > 0 {
		if !bytes.Equal(pattern[:remaining], data[len(data)-remaining:]) {
			return false
		}
	}

	return true
}

// containsSensitiveContent checks if a string contains sensitive patterns
func (v *DefaultKeyValidator) containsSensitiveContent(value string) bool {
	sensitivePatterns := []string{
		"password", "secret", "key", "token", "credential",
		"admin", "root", "auth", "login", "session",
		"/etc/passwd", "/proc/", "C:\\Windows\\",
		"127.0.0.1", "localhost", "192.168.", "10.0.0.",
	}

	valueLower := strings.ToLower(value)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(valueLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}
