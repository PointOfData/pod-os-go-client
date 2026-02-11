// Package log provides an injectable logging interface for the pod-os-go-client.
// It supports structured logging with key-value pairs and level-based filtering
// for high-performance, low-latency use cases.
package log

// Level represents the minimum log level to emit.
// Higher values enable more verbose logging.
type Level int

const (
	LevelDisabled Level = iota
	LevelError
	LevelWarn
	LevelInfo
	LevelDebug
)

// Logger defines the logging interface for the pod-os-go-client.
// Implementations should check Enabled(level) before allocating or formatting
// to avoid overhead when a level is disabled.
type Logger interface {
	// Enabled reports whether the given level is enabled.
	// Callers should use this to avoid allocation when the level is disabled.
	Enabled(level Level) bool

	Debug(msg string, keyvals ...any)
	Info(msg string, keyvals ...any)
	Warn(msg string, keyvals ...any)
	Error(msg string, keyvals ...any)

	// With returns a new Logger with the given keyvals attached to all log records.
	With(keyvals ...any) Logger
}

// NoOpLogger is a no-op implementation that discards all log output.
// Use when Logger is nil or when zero overhead is required.
type NoOpLogger struct{}

// Enabled returns false for all levels.
func (NoOpLogger) Enabled(Level) bool { return false }

// Debug is a no-op.
func (NoOpLogger) Debug(string, ...any) {}

// Info is a no-op.
func (NoOpLogger) Info(string, ...any) {}

// Warn is a no-op.
func (NoOpLogger) Warn(string, ...any) {}

// Error is a no-op.
func (NoOpLogger) Error(string, ...any) {}

// With returns the same NoOpLogger.
func (NoOpLogger) With(...any) Logger { return NoOpLogger{} }

// LoggerOrNoOp returns the given logger if non-nil, otherwise NoOpLogger.
// Use this to avoid nil checks when resolving logger from config.
func LoggerOrNoOp(l Logger) Logger {
	if l == nil {
		return NoOpLogger{}
	}
	return l
}

// LoggerFromConfig resolves the effective logger from config.
// If Logger is non-nil, returns it. If LogLevel > 0, creates a default SlogLogger.
// Otherwise returns NoOpLogger.
func LoggerFromConfig(l Logger, level Level) Logger {
	if l != nil {
		return l
	}
	if level > LevelDisabled {
		return NewSlogLogger(level, nil)
	}
	return NoOpLogger{}
}
