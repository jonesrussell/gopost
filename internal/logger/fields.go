package logger

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// String creates a field with a string value.
// Example: logger.Info("user logged in", String("username", "john"))
func String(key, val string) Field {
	return zap.String(key, val)
}

// Int creates a field with an int value.
// Example: logger.Info("items processed", Int("count", 42))
func Int(key string, val int) Field {
	return zap.Int(key, val)
}

// Int64 creates a field with an int64 value.
// Example: logger.Info("large number", Int64("value", 9223372036854775807))
func Int64(key string, val int64) Field {
	return zap.Int64(key, val)
}

// Uint creates a field with a uint value.
// Example: logger.Info("unsigned value", Uint("count", 100))
func Uint(key string, val uint) Field {
	return zap.Uint(key, val)
}

// Uint64 creates a field with a uint64 value.
// Example: logger.Info("large unsigned value", Uint64("value", 18446744073709551615))
func Uint64(key string, val uint64) Field {
	return zap.Uint64(key, val)
}

// Float64 creates a field with a float64 value.
// Example: logger.Info("measurement", Float64("temperature", 23.5))
func Float64(key string, val float64) Field {
	return zap.Float64(key, val)
}

// Bool creates a field with a boolean value.
// Example: logger.Info("feature status", Bool("enabled", true))
func Bool(key string, val bool) Field {
	return zap.Bool(key, val)
}

// Duration creates a field with a time.Duration value.
// The duration is formatted as a string (e.g., "1s", "100ms").
// Example: logger.Info("request completed", Duration("elapsed", time.Second))
func Duration(key string, val time.Duration) Field {
	return zap.Duration(key, val)
}

// Time creates a field with a time.Time value.
// The time is formatted according to the logger's time encoding.
// Example: logger.Info("event occurred", Time("timestamp", time.Now()))
func Time(key string, val time.Time) Field {
	return zap.Time(key, val)
}

// Error creates a field for an error value.
// This is a convenience function that calls zap.Error.
// The error is logged with the key "error" and includes the error message.
// Example: logger.Error("operation failed", Error(err))
func Error(err error) Field {
	return zap.Error(err)
}

// NamedError creates a field for an error value with a custom key.
// Use this when you want to log multiple errors or use a custom field name.
// Example: logger.Error("validation failed", NamedError("validation_error", err))
func NamedError(key string, err error) Field {
	return zap.NamedError(key, err)
}

// Any creates a field with an arbitrary value.
// The value is serialized using reflection, which may be slower than typed fields.
// Prefer typed field constructors (String, Int, etc.) when possible.
// Example: logger.Info("custom value", Any("data", myStruct))
func Any(key string, val any) Field {
	return zap.Any(key, val)
}

// Strings creates a field with a slice of strings.
// Example: logger.Info("tags", Strings("tags", []string{"go", "logging"}))
func Strings(key string, val []string) Field {
	return zap.Strings(key, val)
}

// Ints creates a field with a slice of integers.
// Example: logger.Info("scores", Ints("scores", []int{95, 87, 92}))
func Ints(key string, val []int) Field {
	return zap.Ints(key, val)
}

// Object creates a field that uses a custom zapcore.ObjectMarshaler.
// This is useful for complex types that need custom serialization.
// Example: logger.Info("user", Object("user", userObject))
func Object(key string, val zapcore.ObjectMarshaler) Field {
	return zap.Object(key, val)
}

// Reflect creates a field that uses reflection to serialize the value.
// This is similar to Any but provides more control over serialization.
// Example: logger.Info("complex object", Reflect("data", myStruct))
func Reflect(key string, val any) Field {
	return zap.Reflect(key, val)
}

// Stack creates a field that captures a stack trace.
// Useful for debugging and error reporting.
// The stack trace is stored with the key "stacktrace".
// Example: logger.Error("unexpected error", Error(err), Stack())
func Stack() Field {
	return zap.Stack("stacktrace")
}

// StackSkip creates a field that captures a stack trace, skipping the given number of frames.
// This is useful when you want to hide internal logging code from the stack trace.
// Example: logger.Error("error", Error(err), StackSkip("", 2))
func StackSkip(key string, skip int) Field {
	return zap.StackSkip(key, skip)
}

// ByteString creates a field with a byte slice that should be logged as a string.
// Useful for logging binary data that should be displayed as text.
// Example: logger.Info("payload", ByteString("data", []byte("hello")))
func ByteString(key string, val []byte) Field {
	return zap.ByteString(key, val)
}

// Binary creates a field with binary data that will be base64-encoded.
// Example: logger.Info("binary data", Binary("data", []byte{0x01, 0x02, 0x03}))
func Binary(key string, val []byte) Field {
	return zap.Binary(key, val)
}
