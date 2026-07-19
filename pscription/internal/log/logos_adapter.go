package log

import "github.com/goodblaster/logos"

// logosAdapter adapts the logos library to implement our Logger interface.
// This is the only file in the codebase (besides main.go) that imports logos.
type logosAdapter struct {
	// Wrap logos.Logger to capture contextual loggers
	// returned by logos.WithError() and logos.With()
	impl logos.Logger
}

// LogosAdapter creates a new Logger backed by a logos.Logger instance.
// This function should only be called from main.go during application startup.
func LogosAdapter(logosLogger logos.Logger) Logger {
	return &logosAdapter{impl: logosLogger}
}

func (l *logosAdapter) Debug(a ...any) {
	l.impl.Debug(a...)
}

func (l *logosAdapter) Info(a ...any) {
	l.impl.Info(a...)
}

func (l *logosAdapter) Warn(a ...any) {
	l.impl.Warn(a...)
}

func (l *logosAdapter) Error(a ...any) {
	l.impl.Error(a...)
}

func (l *logosAdapter) Fatal(a ...any) {
	l.impl.Fatal(a...)
}

func (l *logosAdapter) WithError(err error) Logger {
	// logos.WithError returns a contextual logger
	return &logosAdapter{impl: l.impl.WithError(err)}
}

func (l *logosAdapter) With(key string, value any) Logger {
	// logos.With returns a contextual logger
	return &logosAdapter{impl: l.impl.With(key, value)}
}

func (l *logosAdapter) Print(a ...any) {
	l.impl.Print(a...)
}

// logosDefault delegates to the logos package-level default logger AT CALL
// TIME. It is this package's starting default so that a host application
// (e.g. parch) that configures logos.SetDefaultLogger gets this library's
// logs without needing access to this internal package. Binaries in this
// module (rx) may still override via SetDefault.
type logosDefault struct{}

func (logosDefault) Debug(a ...any) { logos.Debug(a...) }
func (logosDefault) Info(a ...any)  { logos.Info(a...) }
func (logosDefault) Warn(a ...any)  { logos.Warn(a...) }
func (logosDefault) Error(a ...any) { logos.Error(a...) }
func (logosDefault) Fatal(a ...any) { logos.Fatal(a...) }
func (logosDefault) Print(a ...any) { logos.Print(a...) }
func (logosDefault) WithError(err error) Logger {
	return &logosAdapter{impl: logos.WithError(err)}
}
func (logosDefault) With(key string, value any) Logger {
	return &logosAdapter{impl: logos.With(key, value)}
}
