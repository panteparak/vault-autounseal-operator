package client

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/panteparak/vault-autounseal-operator/pkg/core/types"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		tlsSkipVerify bool
		timeout       time.Duration
		expectError   bool
	}{
		{
			name:          "valid configuration",
			url:           "http://localhost:8200",
			tlsSkipVerify: false,
			timeout:       5 * time.Second,
			expectError:   false,
		},
		{
			name:          "empty URL should fail",
			url:           "",
			tlsSkipVerify: false,
			timeout:       5 * time.Second,
			expectError:   true,
		},
		{
			name:          "invalid URL scheme should fail",
			url:           "ftp://localhost:8200",
			tlsSkipVerify: false,
			timeout:       5 * time.Second,
			expectError:   true,
		},
		{
			name:          "very short timeout should fail",
			url:           "http://localhost:8200",
			tlsSkipVerify: false,
			timeout:       0,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.url, tt.tlsSkipVerify, tt.timeout)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				if client != nil {
					client.Close()
				}
			}
		})
	}
}

func TestClient_IsSealed(t *testing.T) {
	// Create a mock Vault server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sys/seal-status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"sealed": true, "progress": 0, "threshold": 3}`))
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, true, 5*time.Second)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	sealed, err := client.IsSealed(ctx)

	assert.NoError(t, err)
	assert.True(t, sealed)
}

func TestClient_HealthCheck(t *testing.T) {
	// Create a mock Vault server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sys/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"initialized": true, "sealed": false, "standby": false}`))
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, true, 5*time.Second)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	health, err := client.HealthCheck(ctx)

	assert.NoError(t, err)
	assert.NotNil(t, health)
	assert.True(t, health.Initialized)
	assert.False(t, health.Sealed)
}

func TestClient_SubmitSingleKey(t *testing.T) {
	// Create a mock Vault server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sys/unseal" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"sealed": false, "progress": 3, "threshold": 3}`))
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, true, 5*time.Second)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	validKey := base64.StdEncoding.EncodeToString([]byte("test-key"))

	status, err := client.SubmitSingleKey(ctx, validKey, 1)

	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.False(t, status.Sealed)
}

func TestClient_SubmitSingleKey_InvalidBase64(t *testing.T) {
	client, err := NewClient("http://localhost:8200", true, 5*time.Second)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	invalidKey := "not-valid-base64!@#"

	_, err = client.SubmitSingleKey(ctx, invalidKey, 1)

	assert.Error(t, err)
	assert.True(t, types.IsValidationError(err))
}

func TestClient_Close(t *testing.T) {
	client, err := NewClient("http://localhost:8200", true, 5*time.Second)
	require.NoError(t, err)

	assert.False(t, client.IsClosed())

	err = client.Close()
	assert.NoError(t, err)
	assert.True(t, client.IsClosed())

	// Closing again should not error
	err = client.Close()
	assert.NoError(t, err)
}

func TestClient_ClosedClientOperations(t *testing.T) {
	client, err := NewClient("http://localhost:8200", true, 5*time.Second)
	require.NoError(t, err)

	// Close the client
	client.Close()

	ctx := context.Background()

	// All operations should return errors when client is closed
	_, err = client.IsSealed(ctx)
	assert.Error(t, err)

	_, err = client.GetSealStatus(ctx)
	assert.Error(t, err)

	_, err = client.IsInitialized(ctx)
	assert.Error(t, err)

	_, err = client.HealthCheck(ctx)
	assert.Error(t, err)
}

func TestDefaultFactory(t *testing.T) {
	factory := &DefaultFactory{}

	client, err := factory.NewClient("http://localhost:8200", true, 5*time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, client)

	if client != nil {
		client.Close()
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				URL:        "http://localhost:8200",
				Timeout:    5 * time.Second,
				MaxRetries: 3,
			},
			expectError: false,
		},
		{
			name: "empty URL",
			config: &Config{
				URL:        "",
				Timeout:    5 * time.Second,
				MaxRetries: 3,
			},
			expectError: true,
		},
		{
			name: "invalid URL scheme",
			config: &Config{
				URL:        "ftp://localhost:8200",
				Timeout:    5 * time.Second,
				MaxRetries: 3,
			},
			expectError: true,
		},
		{
			name: "too short timeout",
			config: &Config{
				URL:        "http://localhost:8200",
				Timeout:    0,
				MaxRetries: 3,
			},
			expectError: true,
		},
		{
			name: "negative max retries",
			config: &Config{
				URL:        "http://localhost:8200",
				Timeout:    5 * time.Second,
				MaxRetries: -1,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
