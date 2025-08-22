package strategy

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/vault/api"

	"github.com/panteparak/vault-autounseal-operator/pkg/core/types"
)

// DefaultUnsealStrategy implements the standard unsealing approach
type DefaultUnsealStrategy struct {
	validator types.KeyValidator
	metrics   types.ClientMetrics
}

// NewDefaultUnsealStrategy creates a new default unseal strategy
func NewDefaultUnsealStrategy(validator types.KeyValidator, metrics types.ClientMetrics) *DefaultUnsealStrategy {
	return &DefaultUnsealStrategy{
		validator: validator,
		metrics:   metrics,
	}
}

// Unseal implements the UnsealStrategy interface
func (s *DefaultUnsealStrategy) Unseal(
	ctx context.Context,
	client types.VaultClient,
	keys []string,
	threshold int,
) (*api.SealStatusResponse, error) {
	start := time.Now()

	// Validate inputs first
	if err := s.validator.ValidateKeys(keys, threshold); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if already unsealed
	status, err := client.GetSealStatus(ctx)
	if err != nil {
		return nil, types.NewVaultError("get-seal-status", "unknown", err, true)
	}

	if !status.Sealed {
		if s.metrics != nil {
			s.metrics.RecordUnsealAttempt("unknown", true, time.Since(start))
		}
		return status, nil
	}

	// Submit keys up to threshold
	keysToSubmit := keys
	if len(keys) > threshold {
		keysToSubmit = keys[:threshold]
	}

	lastStatus, err := s.submitKeys(ctx, client, keysToSubmit)

	if s.metrics != nil {
		s.metrics.RecordUnsealAttempt("unknown", err == nil && !lastStatus.Sealed, time.Since(start))
	}

	return lastStatus, err
}

// submitKeys submits unseal keys one by one
func (s *DefaultUnsealStrategy) submitKeys(
	ctx context.Context,
	client types.VaultClient,
	keys []string,
) (*api.SealStatusResponse, error) {
	var lastStatus *api.SealStatusResponse

	for i, key := range keys {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled during unseal operation: %w", ctx.Err())
		default:
		}

		status, err := s.submitSingleKey(ctx, client, key, i+1)
		if err != nil {
			return nil, &types.UnsealError{
				Endpoint: "unknown",
				KeyIndex: i,
				Err:      err,
			}
		}

		lastStatus = status

		// Stop if unsealed
		if !status.Sealed {
			break
		}

		// Add small delay between key submissions
		time.Sleep(100 * time.Millisecond)
	}

	return lastStatus, nil
}

// submitSingleKey submits a single unseal key
func (s *DefaultUnsealStrategy) submitSingleKey(
	ctx context.Context,
	client types.VaultClient,
	key string,
	index int,
) (*api.SealStatusResponse, error) {
	// Check if client implements our extended interface
	if extendedClient, ok := client.(interface {
		SubmitSingleKey(ctx context.Context, encodedKey string, keyIndex int) (*api.SealStatusResponse, error)
	}); ok {
		return extendedClient.SubmitSingleKey(ctx, key, index)
	}

	// Fallback for interface-only clients - use regular unseal with single key
	return client.Unseal(ctx, []string{key}, 3)
}

// ParallelUnsealStrategy attempts to unseal multiple vault instances in parallel
type ParallelUnsealStrategy struct {
	baseStrategy   types.UnsealStrategy
	maxConcurrency int
}

// NewParallelUnsealStrategy creates a strategy that can handle multiple instances concurrently
func NewParallelUnsealStrategy(baseStrategy types.UnsealStrategy, maxConcurrency int) *ParallelUnsealStrategy {
	if maxConcurrency <= 0 {
		maxConcurrency = 5 // Default reasonable limit
	}
	return &ParallelUnsealStrategy{
		baseStrategy:   baseStrategy,
		maxConcurrency: maxConcurrency,
	}
}

// Unseal implements the UnsealStrategy interface with parallel processing
func (s *ParallelUnsealStrategy) Unseal(
	ctx context.Context,
	client types.VaultClient,
	keys []string,
	threshold int,
) (*api.SealStatusResponse, error) {
	// For single instance, just delegate to base strategy
	return s.baseStrategy.Unseal(ctx, client, keys, threshold)
}

// RetryUnsealStrategy wraps another strategy with retry logic
type RetryUnsealStrategy struct {
	baseStrategy types.UnsealStrategy
	retryPolicy  types.RetryPolicy
}

// NewRetryUnsealStrategy creates a strategy with retry capabilities
func NewRetryUnsealStrategy(baseStrategy types.UnsealStrategy, retryPolicy types.RetryPolicy) *RetryUnsealStrategy {
	return &RetryUnsealStrategy{
		baseStrategy: baseStrategy,
		retryPolicy:  retryPolicy,
	}
}

// Unseal implements the UnsealStrategy interface with retry logic
func (s *RetryUnsealStrategy) Unseal(
	ctx context.Context,
	client types.VaultClient,
	keys []string,
	threshold int,
) (*api.SealStatusResponse, error) {
	var lastErr error

	for attempt := 0; attempt < s.retryPolicy.MaxAttempts(); attempt++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled during retry attempt %d: %w", attempt, ctx.Err())
		default:
		}

		result, err := s.baseStrategy.Unseal(ctx, client, keys, threshold)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// For non-retryable errors, return immediately without retry
		if !types.IsRetryableError(err) {
			return nil, err
		}

		// If we've reached the max attempts, break out of the loop
		if attempt >= s.retryPolicy.MaxAttempts()-1 {
			break
		}

		// Only retry if the policy says we should
		if !s.retryPolicy.ShouldRetry(err, attempt) {
			break
		}

		delay := s.retryPolicy.NextDelay(attempt)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled during retry delay: %w", ctx.Err())
		case <-time.After(delay):
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", s.retryPolicy.MaxAttempts(), lastErr)
}

// DefaultRetryPolicy provides sensible retry defaults
type DefaultRetryPolicy struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

// NewDefaultRetryPolicy creates a retry policy with default settings
func NewDefaultRetryPolicy() *DefaultRetryPolicy {
	return &DefaultRetryPolicy{
		maxAttempts: 3,
		baseDelay:   1 * time.Second,
		maxDelay:    10 * time.Second,
	}
}

// ShouldRetry implements RetryPolicy interface
func (p *DefaultRetryPolicy) ShouldRetry(err error, attempt int) bool {
	if attempt >= p.maxAttempts-1 {
		return false
	}

	// Retry on retryable errors
	return types.IsRetryableError(err)
}

// NextDelay implements RetryPolicy interface with exponential backoff
func (p *DefaultRetryPolicy) NextDelay(attempt int) time.Duration {
	// Prevent integer overflow by capping the attempt number
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 30 { // 2^30 is large enough for practical purposes
		attempt = 30
	}
	delay := p.baseDelay * time.Duration(1<<attempt) // Exponential backoff
	if delay > p.maxDelay {
		delay = p.maxDelay
	}
	return delay
}

// MaxAttempts implements RetryPolicy interface
func (p *DefaultRetryPolicy) MaxAttempts() int {
	return p.maxAttempts
}
