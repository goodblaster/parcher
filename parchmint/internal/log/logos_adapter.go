package log

import "github.com/goodblaster/logos"

// logosAdapter adapts the logos library to the Logger interface. This is
// the only file in the module (besides main.go) that imports logos.
type logosAdapter struct {
	impl logos.Logger
}

// LogosAdapter wraps a concrete logos.Logger as a Logger. Binaries that
// want an explicit logger (rather than the logos package default) call
// SetDefault(LogosAdapter(logger)) at startup.
func LogosAdapter(logosLogger logos.Logger) Logger {
	return &logosAdapter{impl: logosLogger}
}

func (l *logosAdapter) Debug(a ...any) { l.impl.Debug(a...) }
func (l *logosAdapter) Info(a ...any)  { l.impl.Info(a...) }
func (l *logosAdapter) Warn(a ...any)  { l.impl.Warn(a...) }
func (l *logosAdapter) Error(a ...any) { l.impl.Error(a...) }
func (l *logosAdapter) Fatal(a ...any) { l.impl.Fatal(a...) }
func (l *logosAdapter) Print(a ...any) { l.impl.Print(a...) }

func (l *logosAdapter) WithError(err error) Logger {
	return &logosAdapter{impl: l.impl.WithError(err)}
}

func (l *logosAdapter) With(key string, value any) Logger {
	return &logosAdapter{impl: l.impl.With(key, value)}
}

// logosDefault delegates to the logos package-level default logger AT CALL
// TIME. It is this package's starting default, so a binary that calls
// logos.SetDefaultLogger — as parch does — routes this library's logs
// without ever touching this internal package.
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
