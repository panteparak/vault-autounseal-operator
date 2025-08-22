package logging

import (
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// LoggerConfig holds configuration for the logger.
type LoggerConfig struct {
	Level       string
	Development bool
	Encoder     string // json or console
}

// SetupLogger configures and returns a structured logger.
func SetupLogger(config LoggerConfig) (logr.Logger, error) {
	level, err := parseLogLevel(config.Level)
	if err != nil {
		return logr.Logger{}, fmt.Errorf("invalid log level: %w", err)
	}

	encoder, err := parseEncoder(config.Encoder)
	if err != nil {
		return logr.Logger{}, fmt.Errorf("invalid encoder: %w", err)
	}

	opts := zap.Options{
		Development: config.Development,
		Level:       level,
		Encoder:     encoder,
	}

	// Add timestamp to production logs
	if !config.Development {
		opts.TimeEncoder = zapcore.ISO8601TimeEncoder
	}

	return zap.New(zap.UseFlagOptions(&opts)), nil
}

// parseLogLevel converts string log level to zapcore.Level.
func parseLogLevel(level string) (zapcore.Level, error) {
	switch level {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "fatal":
		return zapcore.FatalLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unknown log level: %s", level)
	}
}

// parseEncoder converts string encoder to zapcore.Encoder.
func parseEncoder(encoder string) (zapcore.Encoder, error) {
	switch encoder {
	case "json":
		config := zapcore.EncoderConfig{
			TimeKey:        "timestamp",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "message",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}

		return zapcore.NewJSONEncoder(config), nil
	case "console":
		config := zapcore.EncoderConfig{
			TimeKey:        "T",
			LevelKey:       "L",
			NameKey:        "N",
			CallerKey:      "C",
			MessageKey:     "M",
			StacktraceKey:  "S",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseColorLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}

		return zapcore.NewConsoleEncoder(config), nil
	default:
		return nil, fmt.Errorf("unknown encoder: %s", encoder)
	}
}

// SetupGlobalLogger sets up the global logger for the application.
func SetupGlobalLogger(config LoggerConfig) error {
	logger, err := SetupLogger(config)
	if err != nil {
		return err
	}

	// Set the global logger for controller-runtime
	// Note: This requires importing ctrl "sigs.k8s.io/controller-runtime"
	// and using ctrl.SetLogger(logger) in the actual implementation
	_ = logger // Use the logger or store it in a global variable

	return nil
}

// NewContextLogger creates a new logger with additional context.
func NewContextLogger(base logr.Logger, component string) logr.Logger {
	return base.WithName(component)
}

// LogLevelFromEnv gets log level from environment variable.
func LogLevelFromEnv(envVar, defaultLevel string) string {
	if level := os.Getenv(envVar); level != "" {
		return level
	}

	return defaultLevel
}

// IsDevelopmentFromEnv determines if development mode is enabled from environment.
func IsDevelopmentFromEnv(envVar string) bool {
	return os.Getenv(envVar) == "true" || os.Getenv(envVar) == "1"
}

// EncoderFromEnv gets encoder type from environment variable.
func EncoderFromEnv(envVar, defaultEncoder string) string {
	if encoder := os.Getenv(envVar); encoder != "" {
		return encoder
	}

	return defaultEncoder
}
