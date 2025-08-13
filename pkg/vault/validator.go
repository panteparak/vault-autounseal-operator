package vault

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
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
		return NewValidationError("keys", keys, "no unseal keys provided")
	}

	if threshold < 1 {
		return NewValidationError("threshold", threshold, "threshold must be at least 1")
	}

	if threshold > len(keys) {
		return NewValidationError("threshold", threshold,
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
		return NewValidationError("key", displayKey, "key cannot be empty")
	}

	if len(key) < v.minKeyLength {
		return NewValidationError("key", displayKey,
			fmt.Sprintf("key length (%d) is below minimum (%d)", len(key), v.minKeyLength))
	}

	if len(key) > v.maxKeyLength {
		return NewValidationError("key", displayKey,
			fmt.Sprintf("key length (%d) exceeds maximum (%d)", len(key), v.maxKeyLength))
	}

	// Validate base64 encoding
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return NewValidationError("key", displayKey, fmt.Sprintf("invalid base64 encoding: %v", err))
	}

	// Check decoded key length
	if len(decoded) == 0 {
		return NewValidationError("key", displayKey, "decoded key cannot be empty")
	}

	// Additional validation for key patterns
	if err := v.validateKeyPattern(decoded); err != nil {
		// The validateKeyPattern error might also contain sensitive data
		if v.containsSensitiveContent(key) {
			return NewValidationError("key", displayKey, "key validation failed")
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
			return NewValidationError("keys", keys,
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
		return NewValidationError("key", decoded, "key cannot be all zeros")
	}

	if allSame && len(decoded) > 1 {
		return NewValidationError("key", decoded, "key cannot have all identical bytes")
	}

	// Check for simple repeating patterns
	if len(decoded) >= 8 {
		// Check for 2-byte patterns
		if hasRepeatingPattern(decoded, 2) {
			return NewValidationError("key", decoded, "key contains weak repeating pattern")
		}
		// Check for 4-byte patterns
		if hasRepeatingPattern(decoded, 4) {
			return NewValidationError("key", decoded, "key contains weak repeating pattern")
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

// StrictKeyValidator provides more stringent validation
type StrictKeyValidator struct {
	*DefaultKeyValidator
	requiredKeyLength int
	allowedPrefixes   []string
	forbiddenStrings  []string
}

// NewStrictKeyValidator creates a validator with strict rules
func NewStrictKeyValidator(requiredLength int) *StrictKeyValidator {
	return &StrictKeyValidator{
		DefaultKeyValidator: NewDefaultKeyValidator(),
		requiredKeyLength:   requiredLength,
		allowedPrefixes:     []string{}, // Empty means allow all
		forbiddenStrings:    []string{"password", "secret", "test", "example", "demo"},
	}
}

// ValidateBase64Key validates with strict rules
func (v *StrictKeyValidator) ValidateBase64Key(key string) error {
	// First run default validation
	if err := v.DefaultKeyValidator.ValidateBase64Key(key); err != nil {
		return err
	}

	// Check required length if specified
	if err := v.validateKeyLength(key); err != nil {
		return err
	}

	// Check allowed prefixes
	if err := v.validateAllowedPrefixes(key); err != nil {
		return err
	}

	// Check forbidden strings
	return v.validateForbiddenStrings(key)
}

// validateKeyLength checks if key matches required length
func (v *StrictKeyValidator) validateKeyLength(key string) error {
	if v.requiredKeyLength <= 0 {
		return nil
	}

	decoded, _ := base64.StdEncoding.DecodeString(key) // Already validated above
	if len(decoded) != v.requiredKeyLength {
		return NewValidationError("key", key,
			fmt.Sprintf("key must be exactly %d bytes when decoded (got %d)",
				v.requiredKeyLength, len(decoded)))
	}
	return nil
}

// validateAllowedPrefixes checks if key has valid prefix
func (v *StrictKeyValidator) validateAllowedPrefixes(key string) error {
	if len(v.allowedPrefixes) == 0 {
		return nil
	}

	for _, prefix := range v.allowedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return nil
		}
	}
	return NewValidationError("key", key,
		fmt.Sprintf("key must start with one of: %v", v.allowedPrefixes))
}

// validateForbiddenStrings checks for forbidden content
func (v *StrictKeyValidator) validateForbiddenStrings(key string) error {
	if len(v.forbiddenStrings) == 0 {
		return nil
	}

	keyLower := strings.ToLower(key)
	decoded, _ := base64.StdEncoding.DecodeString(key) // Already validated above
	decodedLower := strings.ToLower(string(decoded))

	for _, forbidden := range v.forbiddenStrings {
		forbiddenLower := strings.ToLower(forbidden)
		if strings.Contains(keyLower, forbiddenLower) || strings.Contains(decodedLower, forbiddenLower) {
			return NewValidationError("key", key,
				fmt.Sprintf("key contains forbidden string: %s", forbidden))
		}
	}
	return nil
}

// SetAllowedPrefixes sets the allowed key prefixes
func (v *StrictKeyValidator) SetAllowedPrefixes(prefixes []string) {
	v.allowedPrefixes = prefixes
}

// SetForbiddenStrings sets strings that are not allowed in keys
func (v *StrictKeyValidator) SetForbiddenStrings(forbidden []string) {
	v.forbiddenStrings = forbidden
}

// ValidateKeys validates keys using strict rules
func (v *StrictKeyValidator) ValidateKeys(keys []string, threshold int) error {
	if len(keys) == 0 {
		return NewValidationError("keys", keys, "no unseal keys provided")
	}

	if threshold < 1 {
		return NewValidationError("threshold", threshold, "threshold must be at least 1")
	}

	if threshold > len(keys) {
		return NewValidationError("threshold", threshold,
			fmt.Sprintf("threshold (%d) exceeds number of available keys (%d)", threshold, len(keys)))
	}

	// Validate each key using strict validation
	for i, key := range keys {
		if err := v.ValidateBase64Key(key); err != nil {
			return fmt.Errorf("invalid key at index %d: %w", i, err)
		}
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for i, key := range keys {
		if seen[key] {
			return NewValidationError("keys", keys, fmt.Sprintf("duplicate key at index %d", i))
		}
		seen[key] = true
	}

	return nil
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
