package vault

import (
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
	if key == "" {
		return NewValidationError("key", key, "key cannot be empty")
	}

	if len(key) < v.minKeyLength {
		return NewValidationError("key", key,
			fmt.Sprintf("key length (%d) is below minimum (%d)", len(key), v.minKeyLength))
	}

	if len(key) > v.maxKeyLength {
		return NewValidationError("key", key,
			fmt.Sprintf("key length (%d) exceeds maximum (%d)", len(key), v.maxKeyLength))
	}

	// Validate base64 encoding
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return NewValidationError("key", key, fmt.Sprintf("invalid base64 encoding: %v", err))
	}

	// Check decoded key length
	if len(decoded) == 0 {
		return NewValidationError("key", key, "decoded key cannot be empty")
	}

	// Additional validation for key patterns
	if err := v.validateKeyPattern(decoded); err != nil {
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

	return nil
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
	if v.requiredKeyLength > 0 {
		decoded, _ := base64.StdEncoding.DecodeString(key) // Already validated above
		if len(decoded) != v.requiredKeyLength {
			return NewValidationError("key", key,
				fmt.Sprintf("key must be exactly %d bytes when decoded (got %d)",
					v.requiredKeyLength, len(decoded)))
		}
	}

	// Check allowed prefixes
	if len(v.allowedPrefixes) > 0 {
		hasValidPrefix := false
		for _, prefix := range v.allowedPrefixes {
			if strings.HasPrefix(key, prefix) {
				hasValidPrefix = true
				break
			}
		}
		if !hasValidPrefix {
			return NewValidationError("key", key,
				fmt.Sprintf("key must start with one of: %v", v.allowedPrefixes))
		}
	}

	// Check forbidden strings
	keyLower := strings.ToLower(key)
	for _, forbidden := range v.forbiddenStrings {
		if strings.Contains(keyLower, strings.ToLower(forbidden)) {
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
