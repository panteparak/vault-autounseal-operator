package vault

import (
	"fmt"
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
	return fmt.Sprintf("validation failed for field '%s' with value '%v': %s", ve.Field, ve.Value, ve.Message)
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
	switch e := err.(type) {
	case *VaultError:
		return e.IsRetryable()
	case *ConnectionError:
		return e.IsRetryable()
	default:
		return false
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
