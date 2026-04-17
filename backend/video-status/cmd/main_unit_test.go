//go:build unit

package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"video-status/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartHttpApi(t *testing.T) {
	t.Run("server addr reflects configured port", func(t *testing.T) {
		tests := []struct {
			name     string
			httpPort string
			wantAddr string
		}{
			{"default port", "8081", ":8081"},
			{"custom port", "9090", ":9090"},
			{"os-assigned port", "0", ":0"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				server := startHttpApi(test.SilentLogger(), test.NewMockKV(), &Config{HTTPPort: tc.httpPort})
				t.Cleanup(func() { server.Close() })

				assert.Equal(t, tc.wantAddr, server.Addr)
			})
		}
	})

	t.Run("route registration", func(t *testing.T) {
		tests := []struct {
			name       string
			method     string
			path       string
			wantStatus int
		}{
			{"POST on status route returns 405", http.MethodPost, "/jobs/abc/status", http.StatusMethodNotAllowed},
			{"PUT on status route returns 405", http.MethodPut, "/jobs/abc/status", http.StatusMethodNotAllowed},
			{"DELETE on status route returns 405", http.MethodDelete, "/jobs/abc/status", http.StatusMethodNotAllowed},
			{"path missing status segment returns 404", http.MethodGet, "/jobs/abc", http.StatusNotFound},
			{"completely unknown path returns 404", http.MethodGet, "/healthz", http.StatusNotFound},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				server := startHttpApi(test.SilentLogger(), test.NewMockKV(), &Config{HTTPPort: "0"})
				t.Cleanup(func() { server.Close() })

				req := httptest.NewRequest(tc.method, tc.path, nil)
				rec := httptest.NewRecorder()
				server.Handler.ServeHTTP(rec, req)

				assert.Equal(t, tc.wantStatus, rec.Code)
			})
		}
	})

	t.Run("CORS middleware is wired", func(t *testing.T) {
		tests := []struct {
			name            string
			origin          string
			wantStatus      int
			wantAllowOrigin string
		}{
			{
				name:            "OPTIONS from allowed origin returns 204 with CORS header",
				origin:          "http://localhost:5173",
				wantStatus:      http.StatusNoContent,
				wantAllowOrigin: "http://localhost:5173",
			},
			{
				name:       "OPTIONS from disallowed origin returns 403 with no CORS header",
				origin:     "http://evil.com",
				wantStatus: http.StatusForbidden,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				server := startHttpApi(test.SilentLogger(), test.NewMockKV(), &Config{HTTPPort: "0"})
				t.Cleanup(func() { server.Close() })

				req := httptest.NewRequest(http.MethodOptions, "/jobs/abc/status", nil)
				req.Header.Set("Origin", tc.origin)
				rec := httptest.NewRecorder()
				server.Handler.ServeHTTP(rec, req)

				assert.Equal(t, tc.wantStatus, rec.Code)
				assert.Equal(t, tc.wantAllowOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
			})
		}
	})

	t.Run("calls osExit(1) when port is already in use", func(t *testing.T) {
		ln, err := net.Listen("tcp", ":0")
		require.NoError(t, err)
		defer ln.Close()
		port := ln.Addr().(*net.TCPAddr).Port

		exitCode := patchOsExit(t)
		startHttpApi(test.SilentLogger(), test.NewMockKV(), &Config{HTTPPort: fmt.Sprintf("%d", port)})

		require.Eventually(t, func() bool {
			return *exitCode == 1
		}, 2*time.Second, 10*time.Millisecond, "expected osExit(1) to be called")
	})
}

func TestMain(t *testing.T) {
	t.Run("exits with code 1 when NATS is unreachable", func(t *testing.T) {
		t.Setenv("NATS_URL", "nats://localhost:1")
		exitCode := patchOsExit(t)
		done := make(chan struct{})

		go func() {
			defer close(done)
			main()
		}()

		select {
		case <-done:
			assert.Equal(t, 1, *exitCode)
		case <-time.After(5 * time.Second):
			t.Error("main() did not call osExit in time")
		}
	})
}
