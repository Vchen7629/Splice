package observability

import (
	"log/slog"
	"os"
)

// General Structured logger for code
func StructuredLogger(prodMode bool) *slog.Logger {
	level := slog.LevelDebug
	if prodMode {
		level = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})

	return slog.New(h).With("service", "transcoder-worker")
}
