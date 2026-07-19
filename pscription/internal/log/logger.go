package log

import (
	"context"
	"sync"
)

// Logger defines the logging interface needed by this application.
// This interface matches logos.Logger method signatures, allowing us to use
// logos.Logger directly without an adapter.
//
// By defining our own interface, we can:
// 1. Swap logging implementations without changing business logic
// 2. Mock logging in tests easily
// 3. Keep dependencies on third-party loggers isolated
//
// This follows the Go proverb: "Accept interfaces, return structs"
type Logger interface {
	// Basic logging levels - matches logos signature
	Debug(a ...any)
	Info(a ...any)
	Warn(a ...any)
	Error(a ...any)
	Fatal(a ...any)

	// Structured logging with error context
	WithError(err error) Logger

	// Structured logging with key-value pairs
	With(key string, value any) Logger

	// Raw print
	Print(a ...any)
}

// Global default logger. The default delegates to the logos package-level
// logger (see logosDefault), so host applications outside this module can
// direct this library's logs just by configuring logos.
var (
	mu            sync.RWMutex
	defaultLogger Logger = logosDefault{}
)

// SetDefault sets the global default logger.
// This should be called once during application startup in main().
func SetDefault(l Logger) {
	mu.Lock()
	defer mu.Unlock()
	defaultLogger = l
}

// Default returns the current default logger.
func Default() Logger {
	mu.RLock()
	defer mu.RUnlock()
	return defaultLogger
}

// Package-level convenience functions that use the default logger

func Debug(a ...any) {
	Default().Debug(a...)
}

func Info(a ...any) {
	Default().Info(a...)
}

func Warn(a ...any) {
	Default().Warn(a...)
}

func Error(a ...any) {
	Default().Error(a...)
}

func Fatal(a ...any) {
	Default().Fatal(a...)
}

func WithError(err error) Logger {
	return Default().WithError(err)
}

func With(key string, value any) Logger {
	return Default().With(key, value)
}

func Print(a ...any) {
	Default().Print(a...)
}

// Context-based logging (optional, for special cases like per-request loggers)

// contextKey is a private type for context keys to avoid collisions
type contextKey struct{}

var loggerKey = contextKey{}

// WithLogger returns a new context with the logger attached.
func WithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext retrieves the logger from the context.
// If no logger is found, it returns the default logger.
func FromContext(ctx context.Context) Logger {
	if logger, ok := ctx.Value(loggerKey).(Logger); ok {
		return logger
	}
	return Default()
}

// NoopLogger discards everything; SetDefault(NoopLogger{}) silences the
// library entirely.
type NoopLogger struct{}

func (l NoopLogger) Debug(a ...any)                    {}
func (l NoopLogger) Info(a ...any)                     {}
func (l NoopLogger) Warn(a ...any)                     {}
func (l NoopLogger) Error(a ...any)                    {}
func (l NoopLogger) Fatal(a ...any)                    {}
func (l NoopLogger) WithError(err error) Logger        { return l }
func (l NoopLogger) With(key string, value any) Logger { return l }
func (l NoopLogger) Print(a ...any)                    {}
