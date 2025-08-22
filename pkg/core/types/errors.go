package types

import (
	"fmt"
	"strings"
	"time"
)

// VaultError represents a vault-specific error
type VaultError struct {
	Operation string
	Endpoint  string
	Err       error
	Retryable bool
	Timestamp time.Time
}

func (ve *VaultError) Error() string {
	return fmt.Sprintf("vault %s failed for %s: %v", ve.Operation, ve.Endpoint, ve.Err)
}

func (ve *VaultError) Unwrap() error {
	return ve.Err
}

func (ve *VaultError) IsRetryable() bool {
	return ve.Retryable
}

// NewVaultError creates a new VaultError
func NewVaultError(operation, endpoint string, err error, retryable bool) *VaultError {
	return &VaultError{
		Operation: operation,
		Endpoint:  endpoint,
		Err:       err,
		Retryable: retryable,
		Timestamp: time.Now(),
	}
}

// ValidationError represents input validation errors
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (ve *ValidationError) Error() string {
	// Redact sensitive field values for security
	var displayValue interface{}
	if ve.Field == "key" || ve.Field == "keys" || containsSensitiveContent(fmt.Sprintf("%v", ve.Value)) {
		displayValue = "[REDACTED]"
	} else {
		displayValue = ve.Value
	}
	return fmt.Sprintf("validation failed for field '%s' with value '%v': %s", ve.Field, displayValue, ve.Message)
}

// containsSensitiveContent checks if a string contains sensitive patterns
func containsSensitiveContent(value string) bool {
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

// NewValidationError creates a new ValidationError
func NewValidationError(field string, value interface{}, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// UnsealError represents errors during unsealing operations
type UnsealError struct {
	Endpoint   string
	KeyIndex   int
	Err        error
	SealStatus *SealStatusInfo
}

func (ue *UnsealError) Error() string {
	return fmt.Sprintf("unseal failed for %s at key index %d: %v", ue.Endpoint, ue.KeyIndex, ue.Err)
}

func (ue *UnsealError) Unwrap() error {
	return ue.Err
}

// SealStatusInfo provides structured seal status information
type SealStatusInfo struct {
	Sealed        bool
	Progress      int
	Threshold     int
	Version       string
	ClusterName   string
	ClusterID     string
	Initialized   bool
	RecoverySeal  bool
	StorageType   string
	HCPLinkStatus string
}

// ConnectionError represents connection-related errors
type ConnectionError struct {
	Endpoint  string
	Err       error
	Timeout   time.Duration
	Retryable bool
}

func (ce *ConnectionError) Error() string {
	return fmt.Sprintf("connection failed to %s (timeout: %v): %v", ce.Endpoint, ce.Timeout, ce.Err)
}

func (ce *ConnectionError) Unwrap() error {
	return ce.Err
}

func (ce *ConnectionError) IsRetryable() bool {
	return ce.Retryable
}

// TimeoutError represents timeout-related errors
type TimeoutError struct {
	Operation string
	Timeout   time.Duration
	Elapsed   time.Duration
}

func (te *TimeoutError) Error() string {
	return fmt.Sprintf("operation '%s' timed out after %v (elapsed: %v)", te.Operation, te.Timeout, te.Elapsed)
}

// AuthenticationError represents authentication failures
type AuthenticationError struct {
	Endpoint string
	Method   string
	Err      error
}

func (ae *AuthenticationError) Error() string {
	return fmt.Sprintf("authentication failed for %s using method '%s': %v", ae.Endpoint, ae.Method, ae.Err)
}

func (ae *AuthenticationError) Unwrap() error {
	return ae.Err
}

// Error checking helpers
func IsRetryableError(err error) bool {
	// Handle wrapped errors by checking the underlying error chain
	for {
		switch e := err.(type) {
		case *VaultError:
			return e.IsRetryable()
		case *ConnectionError:
			return e.IsRetryable()
		case interface{ Unwrap() error }:
			// If the error implements Unwrap, check the underlying error
			err = e.Unwrap()
			if err == nil {
				return false
			}
			continue
		default:
			return false
		}
	}
}

func IsTimeoutError(err error) bool {
	_, ok := err.(*TimeoutError)
	return ok
}

func IsValidationError(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}

func IsAuthenticationError(err error) bool {
	_, ok := err.(*AuthenticationError)
	return ok
}

func IsVaultError(err error) bool {
	_, ok := err.(*VaultError)
	return ok
}

func IsConnectionError(err error) bool {
	_, ok := err.(*ConnectionError)
	return ok
}

func NewConnectionError(endpoint string, err error, retryable bool) *ConnectionError {
	return &ConnectionError{
		Endpoint:  endpoint,
		Err:       err,
		Retryable: retryable,
	}
}

func NewTimeoutError(operation string, timeout time.Duration) *TimeoutError {
	return &TimeoutError{
		Operation: operation,
		Timeout:   timeout,
	}
}
