package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/vault/api"
)

// Client wraps the HashiCorp Vault client with additional functionality
type Client struct {
	client  *api.Client
	url     string
	timeout time.Duration
}

// NewClient creates a new Vault client
func NewClient(url string, tlsSkipVerify bool, timeout time.Duration) (*Client, error) {
	config := api.DefaultConfig()
	config.Address = url
	config.Timeout = timeout

	if tlsSkipVerify {
		err := config.ConfigureTLS(&api.TLSConfig{
			Insecure: true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS: %w", err)
		}
	}

	// Configure HTTP client with security headers and retry logic
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableKeepAlives: false,
			MaxIdleConns:      10,
			IdleConnTimeout:   30 * time.Second,
		},
	}
	config.HttpClient = httpClient

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Set security headers
	client.SetHeaders(map[string][]string{
		"User-Agent":             {"vault-autounseal-operator/1.0"},
		"X-Content-Type-Options": {"nosniff"},
		"X-Frame-Options":        {"DENY"},
	})

	return &Client{
		client:  client,
		url:     url,
		timeout: timeout,
	}, nil
}

// IsSealed checks if the vault is sealed
func (c *Client) IsSealed(ctx context.Context) (bool, error) {
	status, err := c.client.Sys().SealStatusWithContext(ctx)
	if err != nil {
		return true, fmt.Errorf("failed to get seal status: %w", err)
	}
	return status.Sealed, nil
}

// GetSealStatus returns the current seal status
func (c *Client) GetSealStatus(ctx context.Context) (*api.SealStatusResponse, error) {
	status, err := c.client.Sys().SealStatusWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get seal status: %w", err)
	}
	return status, nil
}

// Unseal attempts to unseal the vault using the provided keys
func (c *Client) Unseal(ctx context.Context, keys []string, threshold int) (*api.SealStatusResponse, error) {
	if err := c.validateUnsealParams(keys, threshold); err != nil {
		return nil, err
	}

	// Check if already unsealed
	status, err := c.GetSealStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !status.Sealed {
		return status, nil
	}

	return c.submitUnsealKeys(ctx, keys[:threshold])
}

func (c *Client) validateUnsealParams(keys []string, threshold int) error {
	if len(keys) == 0 {
		return fmt.Errorf("no unseal keys provided")
	}
	if threshold < 1 {
		return fmt.Errorf("threshold must be at least 1")
	}
	if threshold > len(keys) {
		return fmt.Errorf("threshold exceeds number of available keys")
	}
	return nil
}

func (c *Client) submitUnsealKeys(ctx context.Context, keys []string) (*api.SealStatusResponse, error) {
	var lastStatus *api.SealStatusResponse

	for i, encodedKey := range keys {
		status, err := c.submitSingleKey(ctx, encodedKey, i+1)
		if err != nil {
			return nil, err
		}
		lastStatus = status

		// Check if unsealed
		if !lastStatus.Sealed {
			break
		}
	}

	if lastStatus == nil {
		return nil, fmt.Errorf("no keys were successfully submitted")
	}

	return lastStatus, nil
}

func (c *Client) submitSingleKey(
	ctx context.Context,
	encodedKey string,
	keyIndex int,
) (*api.SealStatusResponse, error) {
	// Decode the base64 key
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 encoding in key %d: %w", keyIndex, err)
	}

	// Submit the key
	status, err := c.client.Sys().UnsealWithContext(ctx, string(key))
	if err != nil {
		return nil, fmt.Errorf("failed to submit unseal key %d: %w", keyIndex, err)
	}

	// Clear the key from memory
	for j := range key {
		key[j] = 0
	}

	return status, nil
}

// IsInitialized checks if the vault is initialized
func (c *Client) IsInitialized(ctx context.Context) (bool, error) {
	initialized, err := c.client.Sys().InitStatusWithContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check initialization status: %w", err)
	}
	return initialized, nil
}

// HealthCheck performs a health check on the vault
func (c *Client) HealthCheck(ctx context.Context) (*api.HealthResponse, error) {
	health, err := c.client.Sys().HealthWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	return health, nil
}
