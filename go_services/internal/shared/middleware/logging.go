package middleware

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// General Structured logger for code
func StructuredLogger(prodMode bool, serviceName string) *slog.Logger {
	level := slog.LevelDebug
	if prodMode {
		level = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})

	return slog.New(h).With("service", serviceName)
}

// logging middleware to track status codes, the url path, and response latency
func ApiRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &wrappedWriter{
			ResponseWriter: w,
			StatusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)
		log.Println(wrapped.StatusCode, r.Method, r.URL.Path, time.Since(start))
	})
}
