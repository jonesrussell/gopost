package logger

import (
	"errors"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name  string
		debug bool
	}{
		{
			name:  "development mode",
			debug: true,
		},
		{
			name:  "production mode",
			debug: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, err := NewLogger(tt.debug)
			if err != nil {
				t.Fatalf("NewLogger() error = %v, want nil", err)
			}
			if log == nil {
				t.Fatal("NewLogger() returned nil logger")
			}

			// Test that logger can be used
			log.Info("test message")

			// Test Sync (errors are acceptable in test environments)
			_ = log.Sync()
		})
	}
}

func TestNewNopLogger(t *testing.T) {
	log := NewNopLogger()
	if log == nil {
		t.Fatal("NewNopLogger() returned nil")
	}

	// Nop logger should not panic on any operation
	log.Debug("debug")
	log.Info("info")
	log.Warn("warn")
	log.Error("error")

	// Test With
	withLogger := log.With(String("key", "value"))
	if withLogger == nil {
		t.Fatal("With() returned nil")
	}

	// Test Sync (errors are acceptable in test environments)
	_ = log.Sync()
}

func TestLoggerLevels(t *testing.T) {
	log, err := NewLogger(true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer log.Sync()

	tests := []struct {
		name string
		fn   func(string, ...Field)
	}{
		{
			name: "Debug",
			fn:   log.Debug,
		},
		{
			name: "Info",
			fn:   log.Info,
		},
		{
			name: "Warn",
			fn:   log.Warn,
		},
		{
			name: "Error",
			fn:   log.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			tt.fn("test message")
			tt.fn("test with fields", String("key", "value"))
		})
	}
}

func TestLoggerStructuredFields(t *testing.T) {
	log, err := NewLogger(true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer log.Sync()

	// Test various field types
	log.Debug("test fields",
		String("string_field", "value"),
		Int("int_field", 42),
		Int64("int64_field", 9223372036854775807),
		Uint("uint_field", 100),
		Uint64("uint64_field", 18446744073709551615),
		Float64("float_field", 3.14),
		Bool("bool_field", true),
		Duration("duration_field", time.Second),
		Time("time_field", time.Now()),
		Error(errors.New("test error")),
		NamedError("custom_error", errors.New("custom")),
		Any("any_field", map[string]any{"key": "value"}),
		Strings("strings_field", []string{"a", "b", "c"}),
		Ints("ints_field", []int{1, 2, 3}),
	)

	// Should not panic
}

func TestLoggerWith(t *testing.T) {
	log, err := NewLogger(true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer log.Sync()

	// Create logger with context
	contextLogger := log.With(
		String("service", "test-service"),
		String("version", "1.0.0"),
	)

	if contextLogger == nil {
		t.Fatal("With() returned nil")
	}

	// Test that context is preserved
	contextLogger.Info("message with context")

	// Test chaining With()
	chainedLogger := contextLogger.With(
		String("request_id", "12345"),
	)

	if chainedLogger == nil {
		t.Fatal("chained With() returned nil")
	}

	chainedLogger.Info("message with chained context")

	// Original logger should not have context
	log.Info("message without context")
}

func TestLoggerWithMultipleFields(t *testing.T) {
	log, err := NewLogger(true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer log.Sync()

	// Test With() with multiple fields
	multiLogger := log.With(
		String("field1", "value1"),
		Int("field2", 2),
		Bool("field3", true),
	)

	multiLogger.Info("message with multiple fields")
}

func TestLoggerSync(t *testing.T) {
	log, err := NewLogger(true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	// Log some messages
	log.Info("message 1")
	log.Info("message 2")

	// Sync should be callable (errors are acceptable in test environments)
	_ = log.Sync()

	// Multiple syncs should be safe
	_ = log.Sync()
}

func TestLoggerErrorField(t *testing.T) {
	log, err := NewLogger(true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer log.Sync()

	testErr := errors.New("test error")

	// Test Error() helper
	log.Error("error message", Error(testErr))

	// Test NamedError() helper
	log.Error("named error message", NamedError("validation_error", testErr))
}

func TestLoggerConcurrent(t *testing.T) {
	log, err := NewLogger(true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer log.Sync()

	// Test concurrent logging
	done := make(chan bool, 10)
	for i := range 10 {
		go func(id int) {
			log.Info("concurrent message",
				Int("goroutine_id", id),
			)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
}

func TestLoggerEmptyMessage(t *testing.T) {
	log, err := NewLogger(true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer log.Sync()

	// Should handle empty messages
	log.Debug("")
	log.Info("")
	log.Warn("")
	log.Error("")
}

func TestLoggerNoFields(t *testing.T) {
	log, err := NewLogger(true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer log.Sync()

	// Should handle messages without fields
	log.Debug("message without fields")
	log.Info("message without fields")
	log.Warn("message without fields")
	log.Error("message without fields")
}

func TestLoggerProductionVsDevelopment(t *testing.T) {
	devLog, err := NewLogger(true)
	if err != nil {
		t.Fatalf("NewLogger(debug=true) error = %v", err)
	}
	defer devLog.Sync()

	prodLog, err := NewLogger(false)
	if err != nil {
		t.Fatalf("NewLogger(debug=false) error = %v", err)
	}
	defer prodLog.Sync()

	// Both should work
	devLog.Info("development log")
	prodLog.Info("production log")

	// Both should support all levels
	devLog.Debug("dev debug")
	prodLog.Debug("prod debug")
}
