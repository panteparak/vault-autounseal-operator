package validation

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/panteparak/vault-autounseal-operator/pkg/core/types"
)

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

	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		// This should not happen as key was validated above, but handle gracefully
		return types.NewValidationError("key", key, "invalid base64 encoding")
	}
	if len(decoded) != v.requiredKeyLength {
		return types.NewValidationError("key", key,
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

	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		// This should not happen as key was validated above, but handle gracefully
		return types.NewValidationError("key", key, "invalid base64 encoding")
	}
	decodedStr := string(decoded)

	for _, prefix := range v.allowedPrefixes {
		if strings.HasPrefix(decodedStr, prefix) {
			return nil
		}
	}
	return types.NewValidationError("key", key,
		fmt.Sprintf("decoded key must start with one of: %v", v.allowedPrefixes))
}

// validateForbiddenStrings checks for forbidden content
func (v *StrictKeyValidator) validateForbiddenStrings(key string) error {
	if len(v.forbiddenStrings) == 0 {
		return nil
	}

	keyLower := strings.ToLower(key)
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		// This should not happen as key was validated above, but handle gracefully
		return types.NewValidationError("key", key, "invalid base64 encoding")
	}
	decodedLower := strings.ToLower(string(decoded))

	for _, forbidden := range v.forbiddenStrings {
		forbiddenLower := strings.ToLower(forbidden)
		if strings.Contains(keyLower, forbiddenLower) || strings.Contains(decodedLower, forbiddenLower) {
			return types.NewValidationError("key", key,
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
		return types.NewValidationError("keys", keys, "no unseal keys provided")
	}

	if threshold < 1 {
		return types.NewValidationError("threshold", threshold, "threshold must be at least 1")
	}

	if threshold > len(keys) {
		return types.NewValidationError("threshold", threshold,
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
			return types.NewValidationError("keys", keys, fmt.Sprintf("duplicate key at index %d", i))
		}
		seen[key] = true
	}

	return nil
}
