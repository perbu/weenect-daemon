package main

import (
	"log/slog"
	"os"
	"strings"
)

// newLogger creates a new slog.Logger with the specified log level
func newLogger(level string) *slog.Logger {
	// Parse log level
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	// Create handler with custom options
	opts := &slog.HandlerOptions{
		Level: slogLevel,
	}

	// Use TextHandler for human-readable output
	handler := slog.NewTextHandler(os.Stdout, opts)

	return slog.New(handler)
}
