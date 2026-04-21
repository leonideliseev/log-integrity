// Package logger creates configured slog loggers.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New builds an slog logger with the requested log level.
func New(level string) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	}))
}

// parseLevel converts a string level into slog level constants.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
