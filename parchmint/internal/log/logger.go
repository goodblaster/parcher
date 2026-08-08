// Package log is parchmint's logging seam: an interface matching
// goodblaster/logos so the library code never imports a logging backend
// directly, and binaries pick the implementation.
package log

import (
	"sync"
)

// Logger matches logos.Logger method signatures, so a logos.Logger can back
// it without an adapter shim in the hot path.
type Logger interface {
	Debug(a ...any)
	Info(a ...any)
	Warn(a ...any)
	Error(a ...any)
	Fatal(a ...any)

	WithError(err error) Logger
	With(key string, value any) Logger

	Print(a ...any)
}

// Global default logger. The default delegates to the logos package-level
// logger (see logosDefault), so a binary that configures
// logos.SetDefaultLogger gets this library's logs with no further wiring.
var (
	mu            sync.RWMutex
	defaultLogger Logger = logosDefault{}
)

// SetDefault sets the global default logger. Call once during startup.
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

func Debug(a ...any) { Default().Debug(a...) }
func Info(a ...any)  { Default().Info(a...) }
func Warn(a ...any)  { Default().Warn(a...) }
func Error(a ...any) { Default().Error(a...) }
func Fatal(a ...any) { Default().Fatal(a...) }
func Print(a ...any) { Default().Print(a...) }

func WithError(err error) Logger        { return Default().WithError(err) }
func With(key string, value any) Logger { return Default().With(key, value) }

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
