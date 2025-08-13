package vault

import (
	"context"
	"time"

	"github.com/hashicorp/vault/api"
)

// VaultClient defines the interface for interacting with Vault
type VaultClient interface {
	// IsSealed checks if the vault is sealed
	IsSealed(ctx context.Context) (bool, error)

	// GetSealStatus returns the current seal status
	GetSealStatus(ctx context.Context) (*api.SealStatusResponse, error)

	// Unseal attempts to unseal the vault using the provided keys
	Unseal(ctx context.Context, keys []string, threshold int) (*api.SealStatusResponse, error)

	// IsInitialized checks if the vault is initialized
	IsInitialized(ctx context.Context) (bool, error)

	// HealthCheck performs a health check on the vault
	HealthCheck(ctx context.Context) (*api.HealthResponse, error)

	// Close closes the client and cleans up resources
	Close() error

	// IsClosed returns true if the client has been closed
	IsClosed() bool
}

// ClientFactory creates vault clients
type ClientFactory interface {
	NewClient(endpoint string, tlsSkipVerify bool, timeout time.Duration) (VaultClient, error)
}

// KeyValidator validates unseal keys
type KeyValidator interface {
	ValidateKeys(keys []string, threshold int) error
	ValidateBase64Key(key string) error
}

// UnsealStrategy defines how to unseal a vault instance
type UnsealStrategy interface {
	Unseal(ctx context.Context, client VaultClient, keys []string, threshold int) (*api.SealStatusResponse, error)
}

// ClientMetrics provides metrics for vault client operations
type ClientMetrics interface {
	RecordUnsealAttempt(endpoint string, success bool, duration time.Duration)
	RecordHealthCheck(endpoint string, success bool, duration time.Duration)
	RecordSealStatusCheck(endpoint string, success bool, duration time.Duration)
}

// RetryPolicy defines retry behavior for vault operations
type RetryPolicy interface {
	ShouldRetry(err error, attempt int) bool
	NextDelay(attempt int) time.Duration
	MaxAttempts() int
}
