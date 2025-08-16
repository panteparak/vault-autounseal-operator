package validation

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/panteparak/vault-autounseal-operator/pkg/core/types"
)

func TestDefaultKeyValidator_ValidateKeys(t *testing.T) {
	validator := NewDefaultKeyValidator()

	validKey1 := base64.StdEncoding.EncodeToString([]byte("valid-test-key-1"))
	validKey2 := base64.StdEncoding.EncodeToString([]byte("valid-test-key-2"))
	validKey3 := base64.StdEncoding.EncodeToString([]byte("valid-test-key-3"))

	tests := []struct {
		name        string
		keys        []string
		threshold   int
		expectError bool
	}{
		{
			name:        "valid keys and threshold",
			keys:        []string{validKey1, validKey2, validKey3},
			threshold:   2,
			expectError: false,
		},
		{
			name:        "empty keys slice",
			keys:        []string{},
			threshold:   1,
			expectError: true,
		},
		{
			name:        "threshold too low",
			keys:        []string{validKey1, validKey2},
			threshold:   0,
			expectError: true,
		},
		{
			name:        "threshold exceeds keys",
			keys:        []string{validKey1, validKey2},
			threshold:   3,
			expectError: true,
		},
		{
			name:        "duplicate keys",
			keys:        []string{validKey1, validKey1, validKey2},
			threshold:   2,
			expectError: true,
		},
		{
			name:        "invalid base64 key",
			keys:        []string{"invalid-base64!@#", validKey2},
			threshold:   2,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateKeys(tt.keys, tt.threshold)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultKeyValidator_ValidateBase64Key(t *testing.T) {
	validator := NewDefaultKeyValidator()

	tests := []struct {
		name        string
		key         string
		expectError bool
	}{
		{
			name:        "valid base64 key",
			key:         base64.StdEncoding.EncodeToString([]byte("valid-test-key")),
			expectError: false,
		},
		{
			name:        "empty key",
			key:         "",
			expectError: true,
		},
		{
			name:        "invalid base64",
			key:         "not-valid-base64!@#",
			expectError: true,
		},
		{
			name:        "all zeros key",
			key:         base64.StdEncoding.EncodeToString([]byte{0, 0, 0, 0}),
			expectError: true,
		},
		{
			name:        "all same bytes",
			key:         base64.StdEncoding.EncodeToString([]byte{65, 65, 65, 65}),
			expectError: true,
		},
		{
			name:        "repeating pattern",
			key:         base64.StdEncoding.EncodeToString([]byte{1, 2, 1, 2, 1, 2, 1, 2}),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateBase64Key(tt.key)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultKeyValidator_SensitiveContentRedaction(t *testing.T) {
	validator := NewDefaultKeyValidator()

	sensitiveKey := "password123"
	err := validator.ValidateBase64Key(sensitiveKey)

	assert.Error(t, err)
	assert.True(t, types.IsValidationError(err))
	assert.Contains(t, err.Error(), "[REDACTED]")
	assert.NotContains(t, err.Error(), "password123")
}

func TestStrictKeyValidator(t *testing.T) {
	validator := NewStrictKeyValidator(16) // Require 16-byte keys

	// Create a valid 16-byte key
	validKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
	for i := range make([]byte, 16) {
		validKey = base64.StdEncoding.EncodeToString([]byte{byte(i), byte(i + 1), byte(i + 2), byte(i + 3)})
		if len(validKey) >= 16 {
			break
		}
	}

	// Create an invalid length key
	invalidLengthKey := base64.StdEncoding.EncodeToString([]byte("short"))

	tests := []struct {
		name        string
		key         string
		expectError bool
	}{
		{
			name:        "valid length key",
			key:         base64.StdEncoding.EncodeToString([]byte("sixteen-byte-key")),
			expectError: false,
		},
		{
			name:        "invalid length key",
			key:         invalidLengthKey,
			expectError: true,
		},
		{
			name:        "forbidden string in key",
			key:         base64.StdEncoding.EncodeToString([]byte("test-key-sixteen")),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateBase64Key(tt.key)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStrictKeyValidator_AllowedPrefixes(t *testing.T) {
	validator := NewStrictKeyValidator(0) // No length requirement
	validator.SetAllowedPrefixes([]string{"vault-", "prod-"})

	tests := []struct {
		name        string
		key         string
		expectError bool
	}{
		{
			name:        "valid prefix",
			key:         base64.StdEncoding.EncodeToString([]byte("vault-valid-key")),
			expectError: false,
		},
		{
			name:        "another valid prefix",
			key:         base64.StdEncoding.EncodeToString([]byte("prod-valid-key")),
			expectError: false,
		},
		{
			name:        "invalid prefix",
			key:         base64.StdEncoding.EncodeToString([]byte("dev-valid-key")),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateBase64Key(tt.key)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStrictKeyValidator_ForbiddenStrings(t *testing.T) {
	validator := NewStrictKeyValidator(0) // No length requirement
	validator.SetForbiddenStrings([]string{"admin", "root"})

	tests := []struct {
		name        string
		key         string
		expectError bool
	}{
		{
			name:        "clean key",
			key:         base64.StdEncoding.EncodeToString([]byte("clean-key")),
			expectError: false,
		},
		{
			name:        "forbidden string in encoded key",
			key:         base64.StdEncoding.EncodeToString([]byte("admin-key")),
			expectError: true,
		},
		{
			name:        "forbidden string in base64",
			key:         "admin" + base64.StdEncoding.EncodeToString([]byte("key")),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateBase64Key(tt.key)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHasRepeatingPattern(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		patternLen int
		expected   bool
	}{
		{
			name:       "simple 2-byte pattern",
			data:       []byte{1, 2, 1, 2, 1, 2},
			patternLen: 2,
			expected:   true,
		},
		{
			name:       "no pattern",
			data:       []byte{1, 2, 3, 4, 5, 6},
			patternLen: 2,
			expected:   false,
		},
		{
			name:       "insufficient data",
			data:       []byte{1, 2},
			patternLen: 4,
			expected:   false,
		},
		{
			name:       "4-byte pattern",
			data:       []byte{1, 2, 3, 4, 1, 2, 3, 4},
			patternLen: 4,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasRepeatingPattern(tt.data, tt.patternLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}
