package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger defines the interface for structured logging.
// It provides methods for logging at different levels and adding contextual fields.
type Logger interface {
	// Debug logs a message at debug level.
	Debug(msg string, fields ...Field)

	// Info logs a message at info level.
	Info(msg string, fields ...Field)

	// Warn logs a message at warning level.
	Warn(msg string, fields ...Field)

	// Error logs a message at error level.
	Error(msg string, fields ...Field)

	// With returns a new logger with the given fields attached.
	// Fields are added to all subsequent log entries from this logger.
	With(fields ...Field) Logger

	// Sync flushes any buffered log entries.
	// Applications should call Sync before exiting to ensure all logs are written.
	Sync() error
}

// zapLogger is a zap-based implementation of the Logger interface.
type zapLogger struct {
	logger *zap.Logger
}

// Debug logs a message at debug level.
func (l *zapLogger) Debug(msg string, fields ...Field) {
	l.logger.Debug(msg, fields...)
}

// Info logs a message at info level.
func (l *zapLogger) Info(msg string, fields ...Field) {
	l.logger.Info(msg, fields...)
}

// Warn logs a message at warning level.
func (l *zapLogger) Warn(msg string, fields ...Field) {
	l.logger.Warn(msg, fields...)
}

// Error logs a message at error level.
func (l *zapLogger) Error(msg string, fields ...Field) {
	l.logger.Error(msg, fields...)
}

// With returns a new logger with the given fields attached.
func (l *zapLogger) With(fields ...Field) Logger {
	return &zapLogger{
		logger: l.logger.With(fields...),
	}
}

// Sync flushes any buffered log entries.
func (l *zapLogger) Sync() error {
	return l.logger.Sync()
}

// NewLogger creates a new Logger instance.
// If debug is true, it uses a custom development configuration which provides:
// - Human-readable, colorized output with pretty formatting
// - Stack traces for warnings and errors (not for debug/info to reduce noise)
// - More verbose output suitable for development
// - Clean timestamp formatting
// - Pretty-printed structured fields
//
// If debug is false, it uses zap.NewProduction() which provides:
// - JSON-formatted output
// - Optimized for performance
// - Stack traces only for errors and above
// - Suitable for production environments
//
// Returns an error if the logger cannot be created.
func NewLogger(debug bool) (Logger, error) {
	var z *zap.Logger
	var err error

	if debug {
		// Create a custom development config with prettier formatting
		config := zap.NewDevelopmentConfig()

		// Enable color output for log levels
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

		// Use ISO8601 time format (readable: 2025-12-09T19:30:00.000Z)
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

		// Use short caller format (file:line)
		config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

		// Use console encoder for pretty, human-readable output
		config.Encoding = "console"

		// Set development defaults
		config.Development = true
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)

		// Disable sampling in development for all logs to be visible
		config.Sampling = nil

		z, err = config.Build(
			// Add caller skip to show the actual calling function
			zap.AddCallerSkip(0),
			// Add stack traces only for errors and warnings (not for debug/info)
			zap.AddStacktrace(zapcore.WarnLevel),
		)
	} else {
		z, err = zap.NewProduction()
	}

	if err != nil {
		return nil, err
	}

	return &zapLogger{
		logger: z,
	}, nil
}

// NewNopLogger returns a no-op logger that discards all log entries.
// Useful for testing or when logging should be disabled.
func NewNopLogger() Logger {
	return &zapLogger{
		logger: zap.NewNop(),
	}
}

// Field is a type alias for zapcore.Field.
// It represents a key-value pair that can be attached to a log entry.
type Field = zapcore.Field
