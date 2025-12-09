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
// If debug is true, it uses zap.NewDevelopment() which provides:
// - Human-readable, colorized output
// - Stack traces for all log levels
// - More verbose output suitable for development
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
		z, err = zap.NewDevelopment()
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

