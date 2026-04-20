//go:build unit || integration

package test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"shared/middleware"
	"testing"
)

func NewCorsServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return httptest.NewServer(middleware.Cors(mux))
}

func SilentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
