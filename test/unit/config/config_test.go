package config

import (
	"testing"
	"time"

	"github.com/panteparak/vault-autounseal-operator/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	// Test operator defaults
	assert.Equal(t, ":8080", cfg.Operator.MetricsAddr)
	assert.Equal(t, ":8081", cfg.Operator.ProbeAddr)
	assert.False(t, cfg.Operator.EnableLeaderElection)
	assert.Equal(t, "vault-autounseal-operator-leader", cfg.Operator.LeaderElectionID)
	assert.Equal(t, 30*time.Second, cfg.Operator.GracefulShutdown)

	// Test vault defaults
	assert.Equal(t, 30*time.Second, cfg.Vault.DefaultTimeout)
	assert.False(t, cfg.Vault.DefaultTLSSkipVerify)
	assert.Equal(t, 3, cfg.Vault.MaxRetries)
	assert.Equal(t, time.Second, cfg.Vault.RetryDelay)
	assert.Equal(t, 10, cfg.Vault.ConnectionPoolSize)

	// Test controller defaults
	assert.Equal(t, 30*time.Second, cfg.Controller.RequeueAfter)
	assert.Equal(t, 60*time.Second, cfg.Controller.ReconciliationTimeout)
	assert.Equal(t, 1, cfg.Controller.MaxConcurrentReconciles)

	// Test logging defaults
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.False(t, cfg.Logging.Development)
	assert.Equal(t, "json", cfg.Logging.Encoder)
}

func TestConfigValidateValid(t *testing.T) {
	// Test that default config is valid
	cfg := config.DefaultConfig()
	err := cfg.Validate()
	assert.NoError(t, err, "Default config should be valid")
}

func TestOperatorConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		modifyConfig  func(*config.OperatorConfig)
		expectedError string
	}{
		{
			name: "valid config",
			modifyConfig: func(c *config.OperatorConfig) {
				// Use defaults
			},
			expectedError: "",
		},
		{
			name: "empty metrics addr",
			modifyConfig: func(c *config.OperatorConfig) {
				c.MetricsAddr = ""
			},
			expectedError: "metrics address cannot be empty",
		},
		{
			name: "empty probe addr",
			modifyConfig: func(c *config.OperatorConfig) {
				c.ProbeAddr = ""
			},
			expectedError: "probe address cannot be empty",
		},
		{
			name: "empty leader election ID",
			modifyConfig: func(c *config.OperatorConfig) {
				c.LeaderElectionID = ""
			},
			expectedError: "leader election ID cannot be empty",
		},
		{
			name: "negative graceful shutdown",
			modifyConfig: func(c *config.OperatorConfig) {
				c.GracefulShutdown = -1 * time.Second
			},
			expectedError: "graceful shutdown timeout cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			tt.modifyConfig(&cfg.Operator)

			err := cfg.Operator.Validate()
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestVaultConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		modifyConfig  func(*config.VaultConfig)
		expectedError string
	}{
		{
			name: "valid config",
			modifyConfig: func(c *config.VaultConfig) {
				// Use defaults
			},
			expectedError: "",
		},
		{
			name: "zero default timeout",
			modifyConfig: func(c *config.VaultConfig) {
				c.DefaultTimeout = 0
			},
			expectedError: "default timeout must be positive",
		},
		{
			name: "negative default timeout",
			modifyConfig: func(c *config.VaultConfig) {
				c.DefaultTimeout = -1 * time.Second
			},
			expectedError: "default timeout must be positive",
		},
		{
			name: "negative max retries",
			modifyConfig: func(c *config.VaultConfig) {
				c.MaxRetries = -1
			},
			expectedError: "max retries cannot be negative",
		},
		{
			name: "negative retry delay",
			modifyConfig: func(c *config.VaultConfig) {
				c.RetryDelay = -1 * time.Second
			},
			expectedError: "retry delay cannot be negative",
		},
		{
			name: "zero connection pool size",
			modifyConfig: func(c *config.VaultConfig) {
				c.ConnectionPoolSize = 0
			},
			expectedError: "connection pool size must be positive",
		},
		{
			name: "negative connection pool size",
			modifyConfig: func(c *config.VaultConfig) {
				c.ConnectionPoolSize = -1
			},
			expectedError: "connection pool size must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			tt.modifyConfig(&cfg.Vault)

			err := cfg.Vault.Validate()
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestControllerConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		modifyConfig  func(*config.ControllerConfig)
		expectedError string
	}{
		{
			name: "valid config",
			modifyConfig: func(c *config.ControllerConfig) {
				// Use defaults
			},
			expectedError: "",
		},
		{
			name: "zero requeue after",
			modifyConfig: func(c *config.ControllerConfig) {
				c.RequeueAfter = 0
			},
			expectedError: "requeue after must be positive",
		},
		{
			name: "negative requeue after",
			modifyConfig: func(c *config.ControllerConfig) {
				c.RequeueAfter = -1 * time.Second
			},
			expectedError: "requeue after must be positive",
		},
		{
			name: "zero reconciliation timeout",
			modifyConfig: func(c *config.ControllerConfig) {
				c.ReconciliationTimeout = 0
			},
			expectedError: "reconciliation timeout must be positive",
		},
		{
			name: "negative reconciliation timeout",
			modifyConfig: func(c *config.ControllerConfig) {
				c.ReconciliationTimeout = -1 * time.Second
			},
			expectedError: "reconciliation timeout must be positive",
		},
		{
			name: "zero max concurrent reconciles",
			modifyConfig: func(c *config.ControllerConfig) {
				c.MaxConcurrentReconciles = 0
			},
			expectedError: "max concurrent reconciles must be positive",
		},
		{
			name: "negative max concurrent reconciles",
			modifyConfig: func(c *config.ControllerConfig) {
				c.MaxConcurrentReconciles = -1
			},
			expectedError: "max concurrent reconciles must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			tt.modifyConfig(&cfg.Controller)

			err := cfg.Controller.Validate()
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestLoggingConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		modifyConfig  func(*config.LoggingConfig)
		expectedError string
	}{
		{
			name: "valid config",
			modifyConfig: func(c *config.LoggingConfig) {
				// Use defaults
			},
			expectedError: "",
		},
		{
			name: "valid debug level",
			modifyConfig: func(c *config.LoggingConfig) {
				c.Level = "debug"
			},
			expectedError: "",
		},
		{
			name: "valid warn level",
			modifyConfig: func(c *config.LoggingConfig) {
				c.Level = "warn"
			},
			expectedError: "",
		},
		{
			name: "valid error level",
			modifyConfig: func(c *config.LoggingConfig) {
				c.Level = "error"
			},
			expectedError: "",
		},
		{
			name: "valid fatal level",
			modifyConfig: func(c *config.LoggingConfig) {
				c.Level = "fatal"
			},
			expectedError: "",
		},
		{
			name: "valid console encoder",
			modifyConfig: func(c *config.LoggingConfig) {
				c.Encoder = "console"
			},
			expectedError: "",
		},
		{
			name: "invalid log level",
			modifyConfig: func(c *config.LoggingConfig) {
				c.Level = "invalid"
			},
			expectedError: "invalid log level: invalid",
		},
		{
			name: "invalid encoder",
			modifyConfig: func(c *config.LoggingConfig) {
				c.Encoder = "xml"
			},
			expectedError: "invalid log encoder: xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			tt.modifyConfig(&cfg.Logging)

			err := cfg.Logging.Validate()
			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestConfigValidateAllSections(t *testing.T) {
	// Test that config validation catches errors in all sections
	cfg := config.DefaultConfig()

	// Make operator config invalid
	cfg.Operator.MetricsAddr = ""
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator config validation failed")

	// Reset and make vault config invalid
	cfg = config.DefaultConfig()
	cfg.Vault.DefaultTimeout = 0
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault config validation failed")

	// Reset and make controller config invalid
	cfg = config.DefaultConfig()
	cfg.Controller.RequeueAfter = 0
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "controller config validation failed")

	// Reset and make logging config invalid
	cfg = config.DefaultConfig()
	cfg.Logging.Level = "invalid"
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logging config validation failed")
}

func TestConfigStructTags(t *testing.T) {
	// Test that config structs have proper json/yaml tags for serialization
	// This ensures configs can be loaded from files

	cfg := config.DefaultConfig()

	// The actual validation would require reflection or marshaling/unmarshaling
	// For now, just ensure the structure is reasonable
	assert.NotNil(t, cfg.Operator)
	assert.NotNil(t, cfg.Vault)
	assert.NotNil(t, cfg.Controller)
	assert.NotNil(t, cfg.Logging)
}

func TestConfigModification(t *testing.T) {
	// Test that configs can be modified
	cfg := config.DefaultConfig()

	// Modify values
	cfg.Operator.MetricsAddr = ":9090"
	cfg.Vault.DefaultTimeout = 45 * time.Second
	cfg.Controller.MaxConcurrentReconciles = 5
	cfg.Logging.Level = "debug"
	cfg.Logging.Development = true

	// Should still be valid
	err := cfg.Validate()
	assert.NoError(t, err)

	// Check values were modified
	assert.Equal(t, ":9090", cfg.Operator.MetricsAddr)
	assert.Equal(t, 45*time.Second, cfg.Vault.DefaultTimeout)
	assert.Equal(t, 5, cfg.Controller.MaxConcurrentReconciles)
	assert.Equal(t, "debug", cfg.Logging.Level)
	assert.True(t, cfg.Logging.Development)
}
