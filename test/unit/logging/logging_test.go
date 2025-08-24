package logging

import (
	"os"
	"testing"

	"github.com/panteparak/vault-autounseal-operator/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupLogger(t *testing.T) {
	tests := []struct {
		name   string
		config logging.LoggerConfig
		valid  bool
	}{
		{
			name: "valid json logger",
			config: logging.LoggerConfig{
				Level:       "info",
				Development: false,
				Encoder:     "json",
			},
			valid: true,
		},
		{
			name: "valid console logger",
			config: logging.LoggerConfig{
				Level:       "debug",
				Development: true,
				Encoder:     "console",
			},
			valid: true,
		},
		{
			name: "invalid log level",
			config: logging.LoggerConfig{
				Level:       "invalid",
				Development: false,
				Encoder:     "json",
			},
			valid: false,
		},
		{
			name: "invalid encoder",
			config: logging.LoggerConfig{
				Level:       "info",
				Development: false,
				Encoder:     "xml",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := logging.SetupLogger(tt.config)
			if tt.valid {
				assert.NoError(t, err)
				assert.NotNil(t, logger)
				// Test that we can use the logger
				logger.Info("test log message")
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestSetupLoggerAllLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "warning", "error", "fatal"}

	for _, level := range levels {
		t.Run("level_"+level, func(t *testing.T) {
			config := logging.LoggerConfig{
				Level:       level,
				Development: false,
				Encoder:     "json",
			}

			logger, err := logging.SetupLogger(config)
			assert.NoError(t, err)
			assert.NotNil(t, logger)

			// Test logging at the configured level
			logger.Info("test message", "level", level)
		})
	}
}

func TestSetupLoggerDevelopmentMode(t *testing.T) {
	// Test development mode
	devConfig := logging.LoggerConfig{
		Level:       "debug",
		Development: true,
		Encoder:     "console",
	}

	logger, err := logging.SetupLogger(devConfig)
	require.NoError(t, err)
	assert.NotNil(t, logger)

	// Test production mode
	prodConfig := logging.LoggerConfig{
		Level:       "info",
		Development: false,
		Encoder:     "json",
	}

	logger, err = logging.SetupLogger(prodConfig)
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestSetupGlobalLogger(t *testing.T) {
	config := logging.LoggerConfig{
		Level:       "info",
		Development: false,
		Encoder:     "json",
	}

	err := logging.SetupGlobalLogger(config)
	assert.NoError(t, err)

	// Test with invalid config
	invalidConfig := logging.LoggerConfig{
		Level:       "invalid",
		Development: false,
		Encoder:     "json",
	}

	err = logging.SetupGlobalLogger(invalidConfig)
	assert.Error(t, err)
}

func TestNewContextLogger(t *testing.T) {
	config := logging.LoggerConfig{
		Level:       "info",
		Development: false,
		Encoder:     "json",
	}

	baseLogger, err := logging.SetupLogger(config)
	require.NoError(t, err)

	// Create context logger
	contextLogger := logging.NewContextLogger(baseLogger, "test-component")
	assert.NotNil(t, contextLogger)

	// Test logging with context
	contextLogger.Info("test message with context")

	// Create nested context
	nestedLogger := logging.NewContextLogger(contextLogger, "nested")
	nestedLogger.Info("nested context message")
}

func TestLogLevelFromEnv(t *testing.T) {
	envVar := "TEST_LOG_LEVEL"
	defaultLevel := "info"

	// Test with environment variable not set
	level := logging.LogLevelFromEnv(envVar, defaultLevel)
	assert.Equal(t, defaultLevel, level)

	// Test with environment variable set
	os.Setenv(envVar, "debug")
	defer os.Unsetenv(envVar)

	level = logging.LogLevelFromEnv(envVar, defaultLevel)
	assert.Equal(t, "debug", level)

	// Test with empty environment variable
	os.Setenv(envVar, "")
	level = logging.LogLevelFromEnv(envVar, defaultLevel)
	assert.Equal(t, defaultLevel, level)
}

func TestIsDevelopmentFromEnv(t *testing.T) {
	envVar := "TEST_DEVELOPMENT"

	// Test with environment variable not set
	isDev := logging.IsDevelopmentFromEnv(envVar)
	assert.False(t, isDev)

	// Test with "true"
	os.Setenv(envVar, "true")
	defer os.Unsetenv(envVar)

	isDev = logging.IsDevelopmentFromEnv(envVar)
	assert.True(t, isDev)

	// Test with "1"
	os.Setenv(envVar, "1")
	isDev = logging.IsDevelopmentFromEnv(envVar)
	assert.True(t, isDev)

	// Test with "false"
	os.Setenv(envVar, "false")
	isDev = logging.IsDevelopmentFromEnv(envVar)
	assert.False(t, isDev)

	// Test with other value
	os.Setenv(envVar, "other")
	isDev = logging.IsDevelopmentFromEnv(envVar)
	assert.False(t, isDev)
}

func TestEncoderFromEnv(t *testing.T) {
	envVar := "TEST_ENCODER"
	defaultEncoder := "json"

	// Test with environment variable not set
	encoder := logging.EncoderFromEnv(envVar, defaultEncoder)
	assert.Equal(t, defaultEncoder, encoder)

	// Test with environment variable set
	os.Setenv(envVar, "console")
	defer os.Unsetenv(envVar)

	encoder = logging.EncoderFromEnv(envVar, defaultEncoder)
	assert.Equal(t, "console", encoder)

	// Test with empty environment variable
	os.Setenv(envVar, "")
	encoder = logging.EncoderFromEnv(envVar, defaultEncoder)
	assert.Equal(t, defaultEncoder, encoder)
}

func TestLoggerIntegration(t *testing.T) {
	// Test complete logging setup and usage
	config := logging.LoggerConfig{
		Level:       "debug",
		Development: true,
		Encoder:     "console",
	}

	// Setup logger
	logger, err := logging.SetupLogger(config)
	require.NoError(t, err)

	// Create context loggers
	controllerLogger := logging.NewContextLogger(logger, "controller")
	vaultLogger := logging.NewContextLogger(logger, "vault")
	metricsLogger := logging.NewContextLogger(logger, "metrics")

	// Test logging at different levels
	controllerLogger.Info("controller starting", "version", "1.0.0")
	vaultLogger.Info("connecting to vault", "endpoint", "https://vault.example.com:8200")
	metricsLogger.Info("metrics server started", "port", 8080)

	// Test with key-value pairs
	logger.Info("operation completed",
		"duration", "2.5s",
		"success", true,
		"items", 42)

	// Test error logging
	logger.Error(assert.AnError, "operation failed", "reason", "connection timeout")
}

func TestLoggerConfigurationFromEnvironment(t *testing.T) {
	// Test complete configuration from environment variables
	levelEnv := "TEST_LOG_LEVEL_COMPLETE"
	devEnv := "TEST_DEVELOPMENT_COMPLETE"
	encoderEnv := "TEST_ENCODER_COMPLETE"

	// Set environment variables
	os.Setenv(levelEnv, "warn")
	os.Setenv(devEnv, "true")
	os.Setenv(encoderEnv, "console")
	defer func() {
		os.Unsetenv(levelEnv)
		os.Unsetenv(devEnv)
		os.Unsetenv(encoderEnv)
	}()

	// Create config from environment
	config := logging.LoggerConfig{
		Level:       logging.LogLevelFromEnv(levelEnv, "info"),
		Development: logging.IsDevelopmentFromEnv(devEnv),
		Encoder:     logging.EncoderFromEnv(encoderEnv, "json"),
	}

	assert.Equal(t, "warn", config.Level)
	assert.True(t, config.Development)
	assert.Equal(t, "console", config.Encoder)

	// Test logger works with this config
	logger, err := logging.SetupLogger(config)
	require.NoError(t, err)
	assert.NotNil(t, logger)

	logger.Info("test message from environment config")
}
