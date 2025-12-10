package logger_test

import (
	"errors"
	"time"

	"github.com/gopost/integration/internal/logger"
)

func ExampleNewLogger() {
	// Create a development logger (human-readable, colorized output)
	devLogger, err := logger.NewLogger(true)
	if err != nil {
		panic(err)
	}
	defer devLogger.Sync()

	devLogger.Info("Development logger created")
	// Output:
}

func ExampleNewLogger_production() {
	// Create a production logger (JSON format, optimized for performance)
	prodLogger, err := logger.NewLogger(false)
	if err != nil {
		panic(err)
	}
	defer prodLogger.Sync()

	prodLogger.Info("Production logger created")
	// Output:
}

func ExampleLogger_Info() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	log.Info("User logged in",
		logger.String("username", "john"),
		logger.Int("user_id", 123),
	)
	// Output:
}

func ExampleLogger_Error() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	err := errors.New("database connection failed")
	log.Error("Operation failed",
		logger.String("operation", "database_connect"),
		logger.Error(err),
	)
	// Output:
}

func ExampleLogger_With() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	// Create a logger with request context
	requestLogger := log.With(
		logger.String("request_id", "abc-123"),
		logger.String("user_id", "user-456"),
	)

	// All logs from requestLogger will include the request context
	requestLogger.Info("Processing request")
	requestLogger.Info("Request completed")
	// Output:
}

func ExampleLogger_With_chained() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	// Create base logger with service context
	serviceLogger := log.With(
		logger.String("service", "api"),
		logger.String("version", "1.0.0"),
	)

	// Chain additional context for a specific request
	requestLogger := serviceLogger.With(
		logger.String("request_id", "req-789"),
	)

	// This log will include service, version, and request_id
	requestLogger.Info("Handling request")
	// Output:
}

func ExampleString() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	log.Info("User action",
		logger.String("username", "alice"),
		logger.String("action", "login"),
	)
	// Output:
}

func ExampleInt() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	log.Info("Items processed",
		logger.Int("count", 42),
		logger.Int("total", 100),
	)
	// Output:
}

func ExampleDuration() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	start := time.Now()
	// ... do work ...
	duration := time.Since(start)

	log.Info("Operation completed",
		logger.Duration("elapsed", duration),
	)
	// Output:
}

func ExampleError() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	err := errors.New("file not found")
	log.Error("Failed to read file",
		logger.String("filename", "config.json"),
		logger.Error(err),
	)
	// Output:
}

func ExampleLogger_Debug() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	// Debug logs are useful for detailed troubleshooting
	log.Debug("Processing step",
		logger.String("step", "validation"),
		logger.Int("items", 10),
	)
	// Output:
}

func ExampleLogger_Warn() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	// Warn for non-critical issues
	log.Warn("Rate limit approaching",
		logger.Int("current_rate", 90),
		logger.Int("limit", 100),
	)
	// Output:
}

func ExampleLogger_multipleFields() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	// Combine multiple field types
	log.Info("Complex event",
		logger.String("event_type", "user_action"),
		logger.Int("user_id", 123),
		logger.Bool("success", true),
		logger.Duration("duration", 150*time.Millisecond),
		logger.Time("timestamp", time.Now()),
	)
	// Output:
}

func ExampleNewNopLogger() {
	// Use no-op logger in tests or when logging should be disabled
	log := logger.NewNopLogger()

	// These calls will be no-ops (no output, no overhead)
	log.Debug("debug message")
	log.Info("info message")
	log.Warn("warn message")
	log.Error("error message")

	// Sync is safe to call
	_ = log.Sync()
	// Output:
}

func ExampleLogger_structuredLogging() {
	log, _ := logger.NewLogger(true)
	defer log.Sync()

	// Structured logging best practices:
	// 1. Use descriptive field names
	// 2. Keep messages concise, put details in fields
	// 3. Use appropriate log levels

	// Good: structured, searchable
	log.Info("Article posted",
		logger.String("article_id", "abc123"),
		logger.String("title", "Breaking News"),
		logger.String("city", "sudbury"),
		logger.Int("word_count", 500),
	)

	// Avoid: string concatenation in messages
	// log.Info("Article abc123 titled 'Breaking News' posted to sudbury")
	// Output:
}
