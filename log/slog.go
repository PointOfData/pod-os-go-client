package log

import (
	"io"
	"log/slog"
	"os"
)

// SlogLogger adapts log/slog to the Logger interface.
type SlogLogger struct {
	l     *slog.Logger
	level Level
}

// NewSlogLogger creates a Logger backed by slog.
// level: 0=disabled, 1=Error, 2=Warn, 3=Info, 4=Debug.
// If w is nil, os.Stderr is used.
func NewSlogLogger(level Level, w io.Writer) Logger {
	if w == nil {
		w = os.Stderr
	}
	slogLevel := levelToSlog(level)
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slogLevel})
	return &SlogLogger{l: slog.New(handler), level: level}
}

// NewSlogTextLogger creates a Logger with human-readable text output.
func NewSlogTextLogger(level Level, w io.Writer) Logger {
	if w == nil {
		w = os.Stderr
	}
	slogLevel := levelToSlog(level)
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slogLevel})
	return &SlogLogger{l: slog.New(handler), level: level}
}

func levelToSlog(l Level) slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelError
	}
}

// Enabled reports whether the given level is enabled.
func (s *SlogLogger) Enabled(level Level) bool {
	if level == LevelDisabled {
		return false
	}
	return level <= s.level
}

// Debug logs at debug level.
func (s *SlogLogger) Debug(msg string, keyvals ...any) {
	if s.Enabled(LevelDebug) {
		s.l.Debug(msg, keyvals...)
	}
}

// Info logs at info level.
func (s *SlogLogger) Info(msg string, keyvals ...any) {
	if s.Enabled(LevelInfo) {
		s.l.Info(msg, keyvals...)
	}
}

// Warn logs at warn level.
func (s *SlogLogger) Warn(msg string, keyvals ...any) {
	if s.Enabled(LevelWarn) {
		s.l.Warn(msg, keyvals...)
	}
}

// Error logs at error level.
func (s *SlogLogger) Error(msg string, keyvals ...any) {
	if s.Enabled(LevelError) {
		s.l.Error(msg, keyvals...)
	}
}

// With returns a new SlogLogger with the given keyvals attached.
func (s *SlogLogger) With(keyvals ...any) Logger {
	return &SlogLogger{l: s.l.With(keyvals...), level: s.level}
}
