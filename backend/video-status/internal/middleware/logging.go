package middleware

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// wrapper to extend http response writer to expose
// the status codes
type WrappedWriter struct {
	http.ResponseWriter
	StatusCode int
}

func (w *WrappedWriter) WriteHeader(statuscode int) {
	w.ResponseWriter.WriteHeader(statuscode)
	w.StatusCode = statuscode
}

// General Structured logger for code
func StructuredLogger(prodMode bool) *slog.Logger {
	level := slog.LevelDebug
	if prodMode {
		level = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})

	return slog.New(h).With("service", "video-status")
}

// logging middleware to track status codes, the url path, and response latency
func ApiRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &WrappedWriter{
			ResponseWriter: w,
			StatusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)
		log.Println(wrapped.StatusCode, r.Method, r.URL.Path, time.Since(start))
	})
}
