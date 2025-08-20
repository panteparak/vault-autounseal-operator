package validation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/panteparak/vault-autounseal-operator/pkg/unsealing/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// EdgeCaseTestSuite provides advanced edge case testing for validation
type EdgeCaseTestSuite struct {
	suite.Suite
	validator *validation.DefaultKeyValidator
	ctx       context.Context
}

// SetupSuite initializes the test suite
func (suite *EdgeCaseTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.validator = validation.NewDefaultKeyValidator()
}

// TestUnicodeAndSpecialCharacters tests validation with unicode and special characters
func (suite *EdgeCaseTestSuite) TestUnicodeAndSpecialCharacters() {
	tests := []struct {
		name        string
		input       string
		expectError bool
		description string
	}{
		{
			name:        "unicode characters in base64",
			input:       base64.StdEncoding.EncodeToString([]byte("test-ünïcödé-key-123456789")),
			expectError: false,
			description: "valid unicode content should pass",
		},
		{
			name:        "emoji in base64",
			input:       base64.StdEncoding.EncodeToString([]byte("test-🔐🗝️-key-123456789")),
			expectError: false,
			description: "emoji content should pass validation",
		},
		{
			name:        "null bytes in key",
			input:       base64.StdEncoding.EncodeToString(append([]byte("test-key-"), 0x00, 0x00, 0x00)),
			expectError: false,
			description: "null bytes should be handled",
		},
		{
			name:        "control characters",
			input:       base64.StdEncoding.EncodeToString([]byte("test\x01\x02\x03\x1f-key-123456789")),
			expectError: false,
			description: "control characters should be handled",
		},
		{
			name:        "high bit characters",
			input:       base64.StdEncoding.EncodeToString([]byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9, 0xF8}),
			expectError: false,
			description: "high bit characters should pass",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			err := suite.validator.ValidateBase64Key(tt.input)
			if tt.expectError {
				assert.Error(suite.T(), err, tt.description)
			} else {
				assert.NoError(suite.T(), err, tt.description)
			}
		})
	}
}

// TestBoundaryConditions tests various boundary conditions
func (suite *EdgeCaseTestSuite) TestBoundaryConditions() {
	tests := []struct {
		name        string
		keys        []string
		threshold   int
		expectError bool
		description string
	}{
		{
			name:        "threshold exactly equals key count",
			keys:        []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="}, // test1, test2, test3
			threshold:   3,
			expectError: false,
			description: "threshold equal to key count should pass",
		},
		{
			name:        "threshold one less than key count",
			keys:        []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM=", "dGVzdDQ="},
			threshold:   3,
			expectError: false,
			description: "threshold less than key count should pass",
		},
		{
			name:        "single key with threshold 1",
			keys:        []string{"dGVzdDEyMzQ1Njc4OTA="}, // test1234567890
			threshold:   1,
			expectError: false,
			description: "minimal valid configuration should pass",
		},
		{
			name:        "maximum reasonable key count",
			keys:        generateKeys(100),
			threshold:   50,
			expectError: false,
			description: "large key sets should be handled",
		},
		{
			name:        "threshold at maximum int32",
			keys:        []string{"dGVzdA==", "dGVzdA==", "dGVzdA=="},
			threshold:   2147483647, // max int32
			expectError: true,
			description: "extremely high threshold should fail",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			err := suite.validator.ValidateKeys(tt.keys, tt.threshold)
			if tt.expectError {
				assert.Error(suite.T(), err, tt.description)
			} else {
				assert.NoError(suite.T(), err, tt.description)
			}
		})
	}
}

// TestMalformedBase64Variations tests various malformed base64 scenarios
func (suite *EdgeCaseTestSuite) TestMalformedBase64Variations() {
	tests := []struct {
		name        string
		key         string
		expectError bool
		description string
	}{
		{
			name:        "missing padding",
			key:         "dGVzdA", // missing ==
			expectError: true,
			description: "base64 without proper padding should fail",
		},
		{
			name:        "invalid padding",
			key:         "dGVzdA===", // too much padding
			expectError: true,
			description: "base64 with invalid padding should fail",
		},
		{
			name:        "invalid characters",
			key:         "dGVzdA!@#$%^&*()",
			expectError: true,
			description: "base64 with invalid characters should fail",
		},
		{
			name:        "whitespace in key",
			key:         "dGVz dEE=",
			expectError: true,
			description: "base64 with whitespace should fail",
		},
		{
			name:        "newlines in key",
			key:         "dGVz\ndEE=",
			expectError: true,
			description: "base64 with newlines should fail",
		},
		{
			name:        "tabs in key",
			key:         "dGVz\tdEE=",
			expectError: true,
			description: "base64 with tabs should fail",
		},
		{
			name:        "mixed case issue",
			key:         "DgvzdA==", // Valid base64 but different case
			expectError: false,
			description: "valid base64 with different case should pass",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			err := suite.validator.ValidateBase64Key(tt.key)
			if tt.expectError {
				assert.Error(suite.T(), err, tt.description)
			} else {
				assert.NoError(suite.T(), err, tt.description)
			}
		})
	}
}

// TestCryptographicPatterns tests various cryptographic-related edge cases
func (suite *EdgeCaseTestSuite) TestCryptographicPatterns() {
	tests := []struct {
		name        string
		keyData     []byte
		expectError bool
		description string
	}{
		{
			name:        "all bits set",
			keyData:     bytes(0xFF, 32),
			expectError: false,
			description: "all FF bytes should pass",
		},
		{
			name:        "alternating pattern",
			keyData:     alternatingBytes(0xAA, 0x55, 32),
			expectError: true,
			description: "alternating pattern should be detected as weak",
		},
		{
			name:        "increment pattern",
			keyData:     incrementingBytes(32),
			expectError: false,
			description: "incrementing pattern should pass",
		},
		{
			name:        "random-like data",
			keyData:     generateRandomBytes(32),
			expectError: false,
			description: "random data should pass",
		},
		{
			name:        "crypto weak - repeated 4-byte pattern",
			keyData:     repeatedPattern([]byte{0x01, 0x02, 0x03, 0x04}, 8),
			expectError: true,
			description: "repeated 4-byte pattern should be detected",
		},
		{
			name:        "crypto weak - repeated 2-byte pattern",
			keyData:     repeatedPattern([]byte{0xAB, 0xCD}, 16),
			expectError: true,
			description: "repeated 2-byte pattern should be detected",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			key := base64.StdEncoding.EncodeToString(tt.keyData)
			err := suite.validator.ValidateBase64Key(key)
			if tt.expectError {
				assert.Error(suite.T(), err, tt.description)
			} else {
				assert.NoError(suite.T(), err, tt.description)
			}
		})
	}
}

// TestConcurrentEdgeCases tests concurrency with edge case scenarios
func (suite *EdgeCaseTestSuite) TestConcurrentEdgeCases() {
	concurrency := 100
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)
	successes := make(chan bool, concurrency)

	// Test concurrent validation with various edge cases
	edgeCases := []struct {
		keys      []string
		threshold int
		shouldErr bool
	}{
		{[]string{""}, 1, true},                                                    // empty key
		{[]string{"invalid-base64"}, 1, true},                                     // invalid base64
		{[]string{"dGVzdA==", "dGVzdA=="}, 1, true},                              // duplicates
		{generateKeys(10), 5, false},                                              // valid case
		{[]string{"dGVzdDEyMzQ1Njc4OTA="}, 2, true},                              // threshold too high
		{[]string{base64.StdEncoding.EncodeToString(bytes(0x00, 32))}, 1, true}, // all zeros
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Select edge case based on goroutine ID
			testCase := edgeCases[id%len(edgeCases)]

			err := suite.validator.ValidateKeys(testCase.keys, testCase.threshold)
			if err != nil {
				errors <- err
			} else {
				successes <- true
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	close(successes)

	errorCount := len(errors)
	successCount := len(successes)

	suite.T().Logf("Concurrent edge cases: %d errors, %d successes", errorCount, successCount)
	assert.Equal(suite.T(), concurrency, errorCount+successCount, "All goroutines should complete")
}

// TestMemoryExhaustion tests validation under memory pressure
func (suite *EdgeCaseTestSuite) TestMemoryExhaustion() {
	// Test with extremely large keys (within reasonable limits)
	largeKey := base64.StdEncoding.EncodeToString(make([]byte, 10*1024)) // 10KB key

	err := suite.validator.ValidateBase64Key(largeKey)
	assert.Error(suite.T(), err, "Extremely large keys should fail validation")

	// Test with many keys
	manyKeys := make([]string, 1000)
	for i := range manyKeys {
		manyKeys[i] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("key-%d-1234567890", i)))
	}

	err = suite.validator.ValidateKeys(manyKeys, 500)
	assert.NoError(suite.T(), err, "Many unique keys should be valid")
}

// TestTimeBasedEdgeCases tests validation under time pressure
func (suite *EdgeCaseTestSuite) TestTimeBasedEdgeCases() {
	start := time.Now()

	// Perform many validations quickly
	for i := 0; i < 1000; i++ {
		key := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("test-key-%d", i)))
		err := suite.validator.ValidateBase64Key(key)
		require.NoError(suite.T(), err)
	}

	duration := time.Since(start)
	suite.T().Logf("1000 validations completed in %v", duration)
	assert.Less(suite.T(), duration, 5*time.Second, "Validation should be fast")
}

// TestErrorPropagation tests proper error propagation in complex scenarios
func (suite *EdgeCaseTestSuite) TestErrorPropagation() {
	tests := []struct {
		name        string
		keys        []string
		threshold   int
		expectedErr string
		description string
	}{
		{
			name:        "first key invalid",
			keys:        []string{"invalid", "dGVzdDI=", "dGVzdDM="},
			threshold:   2,
			expectedErr: "invalid key at index 0",
			description: "should report first invalid key",
		},
		{
			name:        "middle key invalid",
			keys:        []string{"dGVzdDE=", "invalid", "dGVzdDM="},
			threshold:   2,
			expectedErr: "invalid key at index 1",
			description: "should report middle invalid key",
		},
		{
			name:        "last key invalid",
			keys:        []string{"dGVzdDE=", "dGVzdDI=", "invalid"},
			threshold:   2,
			expectedErr: "invalid key at index 2",
			description: "should report last invalid key",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			err := suite.validator.ValidateKeys(tt.keys, tt.threshold)
			require.Error(suite.T(), err)
			assert.Contains(suite.T(), err.Error(), tt.expectedErr, tt.description)
		})
	}
}

// Helper functions

// generateKeys creates n unique base64 encoded keys
func generateKeys(n int) []string {
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		data := fmt.Sprintf("test-key-%d-1234567890abcdef", i)
		keys[i] = base64.StdEncoding.EncodeToString([]byte(data))
	}
	return keys
}

// bytes creates a byte slice filled with the specified value
func bytes(value byte, count int) []byte {
	result := make([]byte, count)
	for i := range result {
		result[i] = value
	}
	return result
}

// alternatingBytes creates a byte slice with alternating values
func alternatingBytes(val1, val2 byte, count int) []byte {
	result := make([]byte, count)
	for i := range result {
		if i%2 == 0 {
			result[i] = val1
		} else {
			result[i] = val2
		}
	}
	return result
}

// incrementingBytes creates a byte slice with incrementing values
func incrementingBytes(count int) []byte {
	result := make([]byte, count)
	for i := range result {
		result[i] = byte(i % 256)
	}
	return result
}

// generateRandomBytes creates a byte slice with random data
func generateRandomBytes(count int) []byte {
	result := make([]byte, count)
	rand.Read(result)
	return result
}

// repeatedPattern creates a byte slice by repeating a pattern
func repeatedPattern(pattern []byte, repetitions int) []byte {
	result := make([]byte, 0, len(pattern)*repetitions)
	for i := 0; i < repetitions; i++ {
		result = append(result, pattern...)
	}
	return result
}

// TestEdgeCaseTestSuite runs the edge case test suite
func TestEdgeCaseTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping edge case tests in short mode")
	}

	suite.Run(t, new(EdgeCaseTestSuite))
}
