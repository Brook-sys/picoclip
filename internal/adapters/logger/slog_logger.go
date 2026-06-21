package logger

import (
	"log/slog"
	"os"

	"picoclip/internal/core/ports"
)

type SlogLogger struct {
	l *slog.Logger
}

var _ ports.Logger = (*SlogLogger)(nil)

func NewSlogLogger(level string, debug bool) *SlogLogger {
	lvl := slog.LevelInfo
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	if debug {
		lvl = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if debug {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return &SlogLogger{l: slog.New(handler)}
}

func (s *SlogLogger) Debug(msg string, args ...any) { s.l.Debug(msg, args...) }
func (s *SlogLogger) Info(msg string, args ...any)  { s.l.Info(msg, args...) }
func (s *SlogLogger) Warn(msg string, args ...any)  { s.l.Warn(msg, args...) }
func (s *SlogLogger) Error(msg string, args ...any) { s.l.Error(msg, args...) }
