package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"

	"github.com/panteparak/vault-autounseal-operator/pkg/core/types"
)

// Client wraps the HashiCorp Vault client with additional functionality
type Client struct {
	client   *api.Client
	url      string
	timeout  time.Duration
	strategy types.UnsealStrategy
	metrics  types.ClientMetrics
	mu       sync.RWMutex
	closed   bool
}

// Config holds configuration for creating a vault client
type Config struct {
	URL           string
	TLSSkipVerify bool
	Timeout       time.Duration
	Validator     types.KeyValidator
	Strategy      types.UnsealStrategy
	Metrics       types.ClientMetrics
	MaxRetries    int
	RetryDelay    time.Duration
}

// NewClient creates a new Vault client with the given configuration
func NewClient(url string, tlsSkipVerify bool, timeout time.Duration) (*Client, error) {
	config := &Config{
		URL:           url,
		TLSSkipVerify: tlsSkipVerify,
		Timeout:       timeout,
		MaxRetries:    3,
		RetryDelay:    time.Second,
	}
	return NewClientWithConfig(config)
}

// validateConfig validates the client configuration
func validateConfig(config *Config) error {
	if config.URL == "" {
		return types.NewValidationError("url", config.URL, "URL cannot be empty")
	}

	// Basic URL validation
	if !strings.HasPrefix(config.URL, "http://") && !strings.HasPrefix(config.URL, "https://") {
		return types.NewValidationError("url", config.URL, "URL must start with http:// or https://")
	}

	// Reject extremely long URLs
	if len(config.URL) > 2048 {
		return types.NewValidationError("url", config.URL, "URL exceeds maximum length of 2048 characters")
	}

	// Reject extremely small timeouts
	if config.Timeout < time.Millisecond {
		return types.NewValidationError("timeout", config.Timeout, "Timeout must be at least 1 millisecond")
	}

	if config.MaxRetries < 0 {
		return types.NewValidationError("maxRetries", config.MaxRetries, "MaxRetries cannot be negative")
	}

	return nil
}

// NewClientWithConfig creates a new Vault client with advanced configuration
func NewClientWithConfig(config *Config) (*Client, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	vaultConfig := api.DefaultConfig()
	vaultConfig.Address = config.URL
	vaultConfig.Timeout = config.Timeout

	if config.TLSSkipVerify {
		err := vaultConfig.ConfigureTLS(&api.TLSConfig{
			Insecure: true,
		})
		if err != nil {
			return nil, types.NewVaultError("tls-config", config.URL, err, false)
		}
	}

	// Configure HTTP client with security headers and connection pooling
	httpClient := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			DisableKeepAlives:   false,
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
			MaxConnsPerHost:     50,
		},
	}
	vaultConfig.HttpClient = httpClient

	apiClient, err := api.NewClient(vaultConfig)
	if err != nil {
		return nil, types.NewVaultError("client-creation", config.URL, err, false)
	}

	// Set security headers
	apiClient.SetHeaders(map[string][]string{
		"User-Agent":             {"vault-autounseal-operator/2.0"},
		"X-Content-Type-Options": {"nosniff"},
		"X-Frame-Options":        {"DENY"},
		"X-Request-ID":           {fmt.Sprintf("vault-operator-%d", time.Now().UnixNano())},
	})

	client := &Client{
		client:  apiClient,
		url:     config.URL,
		timeout: config.Timeout,
		metrics: config.Metrics,
	}

	return client, nil
}

// IsSealed checks if the vault is sealed
func (c *Client) IsSealed(ctx context.Context) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return true, types.NewVaultError("is-sealed", c.url, fmt.Errorf("client is closed"), false)
	}

	start := time.Now()
	status, err := c.client.Sys().SealStatusWithContext(ctx)

	if c.metrics != nil {
		c.metrics.RecordSealStatusCheck(c.url, err == nil, time.Since(start))
	}

	if err != nil {
		return true, types.NewVaultError("seal-status", c.url, err, true)
	}
	return status.Sealed, nil
}

// GetSealStatus returns the current seal status
func (c *Client) GetSealStatus(ctx context.Context) (*api.SealStatusResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, types.NewVaultError("get-seal-status", c.url, fmt.Errorf("client is closed"), false)
	}

	start := time.Now()
	status, err := c.client.Sys().SealStatusWithContext(ctx)

	if c.metrics != nil {
		c.metrics.RecordSealStatusCheck(c.url, err == nil, time.Since(start))
	}

	if err != nil {
		return nil, types.NewVaultError("seal-status", c.url, err, true)
	}
	return status, nil
}

// Unseal attempts to unseal the vault using the provided keys
func (c *Client) Unseal(ctx context.Context, keys []string, threshold int) (*api.SealStatusResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, types.NewVaultError("unseal", c.url, fmt.Errorf("client is closed"), false)
	}

	// If no strategy is configured, use direct key submission
	if c.strategy == nil {
		return c.unsealDirect(ctx, keys, threshold)
	}

	// Use the configured strategy for unsealing
	return c.strategy.Unseal(ctx, c, keys, threshold)
}

// unsealDirect provides direct unsealing without strategy pattern
func (c *Client) unsealDirect(ctx context.Context, keys []string, threshold int) (*api.SealStatusResponse, error) {
	// Check if already unsealed
	status, err := c.GetSealStatus(ctx)
	if err != nil {
		return nil, err
	}

	if !status.Sealed {
		return status, nil
	}

	// Submit keys up to threshold
	keysToSubmit := keys
	if len(keys) > threshold {
		keysToSubmit = keys[:threshold]
	}

	var lastStatus *api.SealStatusResponse
	for i, key := range keysToSubmit {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled during unseal operation: %w", ctx.Err())
		default:
		}

		currentStatus, err := c.SubmitSingleKey(ctx, key, i+1)
		if err != nil {
			return nil, err
		}

		lastStatus = currentStatus

		// Stop if unsealed
		if !currentStatus.Sealed {
			break
		}

		// Add small delay between key submissions
		time.Sleep(100 * time.Millisecond)
	}

	return lastStatus, nil
}

// SubmitSingleKey submits a single unseal key (used by strategies)
func (c *Client) SubmitSingleKey(ctx context.Context, encodedKey string, keyIndex int) (*api.SealStatusResponse, error) {
	// Validate that the key is valid base64
	_, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, types.NewValidationError("key", encodedKey, fmt.Sprintf("invalid base64 encoding in key %d: %v", keyIndex, err))
	}

	// Submit the base64 encoded key directly (Vault API expects base64)
	status, err := c.client.Sys().UnsealWithContext(ctx, encodedKey)
	if err != nil {
		return nil, types.NewVaultError("unseal-key-submit", c.url, fmt.Errorf("failed to submit unseal key %d: %w", keyIndex, err), true)
	}

	return status, nil
}

// IsInitialized checks if the vault is initialized
func (c *Client) IsInitialized(ctx context.Context) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return false, types.NewVaultError("is-initialized", c.url, fmt.Errorf("client is closed"), false)
	}

	initialized, err := c.client.Sys().InitStatusWithContext(ctx)
	if err != nil {
		return false, types.NewVaultError("init-status", c.url, err, true)
	}
	return initialized, nil
}

// HealthCheck performs a health check on the vault
func (c *Client) HealthCheck(ctx context.Context) (*api.HealthResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, types.NewVaultError("health-check", c.url, fmt.Errorf("client is closed"), false)
	}

	start := time.Now()
	health, err := c.client.Sys().HealthWithContext(ctx)

	if c.metrics != nil {
		c.metrics.RecordHealthCheck(c.url, err == nil, time.Since(start))
	}

	if err != nil {
		return nil, types.NewVaultError("health-check", c.url, err, true)
	}
	return health, nil
}

// Close closes the client and cleans up resources
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	// Clear any sensitive data
	if c.client != nil {
		c.client.ClearToken()
	}

	return nil
}

// URL returns the vault endpoint URL
func (c *Client) URL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.url
}

// Timeout returns the configured timeout
func (c *Client) Timeout() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.timeout
}

// IsClosed returns true if the client has been closed
func (c *Client) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// DefaultFactory implements the ClientFactory interface
type DefaultFactory struct{}

// NewClient implements ClientFactory interface
func (f *DefaultFactory) NewClient(endpoint string, tlsSkipVerify bool, timeout time.Duration) (types.VaultClient, error) {
	return NewClient(endpoint, tlsSkipVerify, timeout)
}
