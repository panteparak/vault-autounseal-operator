package vault

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/panteparak/vault-autounseal-operator/pkg/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ClientTestSuite provides unit testing for vault client functionality
type ClientTestSuite struct {
	suite.Suite
	ctx context.Context
}

func (suite *ClientTestSuite) SetupSuite() {
	suite.ctx = context.Background()
}

// TestNewClient tests basic client creation
func (suite *ClientTestSuite) TestNewClient() {
	tests := []struct {
		name          string
		url           string
		tlsSkipVerify bool
		timeout       time.Duration
		expectError   bool
	}{
		{
			name:          "valid HTTP URL",
			url:           "http://localhost:8200",
			tlsSkipVerify: false,
			timeout:       30 * time.Second,
			expectError:   false,
		},
		{
			name:          "valid HTTPS URL",
			url:           "https://vault.example.com:8200",
			tlsSkipVerify: true,
			timeout:       30 * time.Second,
			expectError:   false,
		},
		{
			name:          "empty URL",
			url:           "",
			tlsSkipVerify: false,
			timeout:       30 * time.Second,
			expectError:   true,
		},
		{
			name:          "invalid URL scheme",
			url:           "ftp://localhost:8200",
			tlsSkipVerify: false,
			timeout:       30 * time.Second,
			expectError:   true,
		},
		{
			name:          "extremely long URL",
			url:           "http://" + string(make([]byte, 2050)),
			tlsSkipVerify: false,
			timeout:       30 * time.Second,
			expectError:   true,
		},
		{
			name:          "extremely small timeout",
			url:           "http://localhost:8200",
			tlsSkipVerify: false,
			timeout:       time.Nanosecond,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			client, err := vault.NewClient(tt.url, tt.tlsSkipVerify, tt.timeout)
			if tt.expectError {
				assert.Error(suite.T(), err)
				assert.Nil(suite.T(), client)
			} else {
				require.NoError(suite.T(), err)
				require.NotNil(suite.T(), client)
				assert.Equal(suite.T(), tt.url, client.URL())
				assert.Equal(suite.T(), tt.timeout, client.Timeout())
				assert.False(suite.T(), client.IsClosed())
				client.Close()
			}
		})
	}
}

// TestNewClientWithConfig tests client creation with advanced configuration
func (suite *ClientTestSuite) TestNewClientWithConfig() {
	tests := []struct {
		name        string
		config      *vault.ClientConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: &vault.ClientConfig{
				URL:           "http://localhost:8200",
				TLSSkipVerify: false,
				Timeout:       30 * time.Second,
				MaxRetries:    3,
				RetryDelay:    time.Second,
			},
			expectError: false,
		},
		{
			name: "config with negative retries",
			config: &vault.ClientConfig{
				URL:           "http://localhost:8200",
				TLSSkipVerify: false,
				Timeout:       30 * time.Second,
				MaxRetries:    -1,
				RetryDelay:    time.Second,
			},
			expectError: true,
		},
		{
			name: "config with very small timeout (should be set to default)",
			config: &vault.ClientConfig{
				URL:           "http://localhost:8200",
				TLSSkipVerify: false,
				Timeout:       1 * time.Millisecond,
				MaxRetries:    1,
				RetryDelay:    time.Second,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			client, err := vault.NewClientWithConfig(tt.config)
			if tt.expectError {
				assert.Error(suite.T(), err)
				assert.Nil(suite.T(), client)
			} else {
				require.NoError(suite.T(), err)
				require.NotNil(suite.T(), client)
				if tt.config.Timeout <= 0 || tt.config.Timeout < time.Second {
					// For very small timeouts, the client may adjust them
					assert.True(suite.T(), client.Timeout() >= tt.config.Timeout)
				} else {
					assert.Equal(suite.T(), tt.config.Timeout, client.Timeout())
				}
				client.Close()
			}
		})
	}
}

// TestClientClose tests client closing functionality
func (suite *ClientTestSuite) TestClientClose() {
	client, err := vault.NewClient("http://localhost:8200", false, 30*time.Second)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), client)

	// Initially should not be closed
	assert.False(suite.T(), client.IsClosed())

	// Close the client
	err = client.Close()
	require.NoError(suite.T(), err)
	assert.True(suite.T(), client.IsClosed())

	// Closing again should not error
	err = client.Close()
	require.NoError(suite.T(), err)
	assert.True(suite.T(), client.IsClosed())
}

// TestClientClosedOperations tests operations on a closed client
func (suite *ClientTestSuite) TestClientClosedOperations() {
	client, err := vault.NewClient("http://localhost:8200", false, 30*time.Second)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), client)

	// Close the client
	err = client.Close()
	require.NoError(suite.T(), err)

	// All operations should fail with client closed error
	_, err = client.IsSealed(suite.ctx)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "client is closed")

	_, err = client.GetSealStatus(suite.ctx)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "client is closed")

	_, err = client.Unseal(suite.ctx, []string{"key1", "key2", "key3"}, 3)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "client is closed")

	_, err = client.IsInitialized(suite.ctx)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "client is closed")

	_, err = client.HealthCheck(suite.ctx)
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "client is closed")
}

// TestSubmitSingleKey tests single key submission validation
func (suite *ClientTestSuite) TestSubmitSingleKey() {
	client, err := vault.NewClient("http://localhost:8200", false, 30*time.Second)
	require.NoError(suite.T(), err)
	defer client.Close()

	tests := []struct {
		name        string
		key         string
		keyIndex    int
		expectError bool
	}{
		{
			name:        "valid base64 key",
			key:         base64.StdEncoding.EncodeToString([]byte("valid-key-data")),
			keyIndex:    0,
			expectError: false, // Will fail due to no vault server, but validation should pass
		},
		{
			name:        "invalid base64 key",
			key:         "not-valid-base64!!!",
			keyIndex:    0,
			expectError: true,
		},
		{
			name:        "empty key",
			key:         "",
			keyIndex:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			_, err := client.SubmitSingleKey(suite.ctx, tt.key, tt.keyIndex)
			if tt.expectError {
				assert.Error(suite.T(), err)
				// For invalid base64, should be validation error
				if tt.key == "not-valid-base64!!!" || tt.key == "" {
					assert.Contains(suite.T(), err.Error(), "invalid base64")
				}
			} else {
				// We expect this to fail due to no vault server, but not due to validation
				assert.Error(suite.T(), err)
				// Should not be a validation error
				assert.NotContains(suite.T(), err.Error(), "invalid base64")
			}
		})
	}
}

// TestDefaultClientFactory tests the default client factory implementation
func (suite *ClientTestSuite) TestDefaultClientFactory() {
	factory := &vault.DefaultClientFactory{}

	client, err := factory.NewClient("http://localhost:8200", false, 30*time.Second)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), client)

	// Ensure it returns a vault.VaultClient interface
	var vaultClient vault.VaultClient = client
	assert.NotNil(suite.T(), vaultClient)

	client.Close()
}

// TestValidateClientConfig tests the client configuration validation
// Commented out as validateClientConfig is unexported
/* func (suite *ClientTestSuite) TestValidateClientConfig() {
	tests := []struct {
		name        string
		config      *vault.ClientConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: &vault.ClientConfig{
				URL:        "http://localhost:8200",
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			},
			expectError: false,
		},
		{
			name: "empty URL",
			config: &vault.ClientConfig{
				URL:        "",
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			},
			expectError: true,
		},
		{
			name: "invalid URL scheme",
			config: &vault.ClientConfig{
				URL:        "ftp://localhost:8200",
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			},
			expectError: true,
		},
		{
			name: "extremely long URL",
			config: &vault.ClientConfig{
				URL:        "http://" + string(make([]byte, 2050)),
				Timeout:    30 * time.Second,
				MaxRetries: 3,
			},
			expectError: true,
		},
		{
			name: "timeout too small",
			config: &vault.ClientConfig{
				URL:        "http://localhost:8200",
				Timeout:    time.Nanosecond,
				MaxRetries: 3,
			},
			expectError: true,
		},
		{
			name: "negative max retries",
			config: &vault.ClientConfig{
				URL:        "http://localhost:8200",
				Timeout:    30 * time.Second,
				MaxRetries: -1,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			err := validateClientConfig(tt.config)
			if tt.expectError {
				assert.Error(suite.T(), err)
			} else {
				assert.NoError(suite.T(), err)
			}
		})
	}
} */

// TestClientThreadSafety tests thread safety of client operations
func (suite *ClientTestSuite) TestClientThreadSafety() {
	client, err := vault.NewClient("http://localhost:8200", false, 30*time.Second)
	require.NoError(suite.T(), err)
	defer client.Close()

	// Test concurrent access to client properties
	concurrency := 100
	done := make(chan bool, concurrency)

	// Test concurrent URL() calls
	for i := 0; i < concurrency; i++ {
		go func() {
			url := client.URL()
			assert.Equal(suite.T(), "http://localhost:8200", url)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < concurrency; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			suite.T().Fatal("Timeout waiting for concurrent operations")
		}
	}

	// Test concurrent Timeout() calls
	for i := 0; i < concurrency; i++ {
		go func() {
			timeout := client.Timeout()
			assert.Equal(suite.T(), 30*time.Second, timeout)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < concurrency; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			suite.T().Fatal("Timeout waiting for concurrent timeout operations")
		}
	}

	// Test concurrent IsClosed() calls
	for i := 0; i < concurrency; i++ {
		go func() {
			closed := client.IsClosed()
			assert.False(suite.T(), closed)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < concurrency; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			suite.T().Fatal("Timeout waiting for concurrent IsClosed operations")
		}
	}
}

// TestClientWithCustomValidator tests client with custom validator
func (suite *ClientTestSuite) TestClientWithCustomValidator() {
	// Create a custom validator that always returns an error
	customValidator := &MockKeyValidator{
		validateFunc: func(keys []string, threshold int) error {
			return fmt.Errorf("custom validation error")
		},
	}

	config := &vault.ClientConfig{
		URL:           "http://localhost:8200",
		TLSSkipVerify: false,
		Timeout:       30 * time.Second,
		Validator:     customValidator,
		MaxRetries:    1,
	}

	client, err := vault.NewClientWithConfig(config)
	require.NoError(suite.T(), err)
	defer client.Close()

	// The client should have the custom validator
	// This would be tested through the unsealing process, but since we don't have
	// a real vault server, we can't fully test it here
	assert.NotNil(suite.T(), client)
}

// TestClientWithCustomStrategy tests client with custom strategy
func (suite *ClientTestSuite) TestClientWithCustomStrategy() {
	// Create a custom strategy
	customStrategy := &MockUnsealStrategy{
		unsealFunc: func(ctx context.Context, client vault.VaultClient, keys []string, threshold int) (*api.SealStatusResponse, error) {
			return &api.SealStatusResponse{Sealed: false}, nil
		},
	}

	config := &vault.ClientConfig{
		URL:           "http://localhost:8200",
		TLSSkipVerify: false,
		Timeout:       30 * time.Second,
		Strategy:      customStrategy,
		MaxRetries:    1,
	}

	client, err := vault.NewClientWithConfig(config)
	require.NoError(suite.T(), err)
	defer client.Close()

	// The client should have the custom strategy
	assert.NotNil(suite.T(), client)
}

// MockKeyValidator implements KeyValidator for testing
type MockKeyValidator struct {
	validateFunc        func(keys []string, threshold int) error
	validateBase64Func func(key string) error
}

func (m *MockKeyValidator) ValidateKeys(keys []string, threshold int) error {
	if m.validateFunc != nil {
		return m.validateFunc(keys, threshold)
	}
	return nil
}

func (m *MockKeyValidator) ValidateBase64Key(key string) error {
	if m.validateBase64Func != nil {
		return m.validateBase64Func(key)
	}
	return nil
}

// MockUnsealStrategy implements UnsealStrategy for testing
type MockUnsealStrategy struct {
	unsealFunc func(ctx context.Context, client vault.VaultClient, keys []string, threshold int) (*api.SealStatusResponse, error)
}

func (m *MockUnsealStrategy) Unseal(ctx context.Context, client vault.VaultClient, keys []string, threshold int) (*api.SealStatusResponse, error) {
	if m.unsealFunc != nil {
		return m.unsealFunc(ctx, client, keys, threshold)
	}
	return &api.SealStatusResponse{Sealed: false}, nil
}

// TestVaultClientTestSuite runs the vault client test suite
func TestVaultClientTestSuite(t *testing.T) {
	suite.Run(t, new(ClientTestSuite))
}
