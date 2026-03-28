//go:build unit

package middleware_test

import (
	"bytes"
	"context"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"video-upload/internal/middleware"

	"github.com/stretchr/testify/assert"
)

func TestWriteHeader(t *testing.T) {
	t.Run("Captures status code properly", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		wrapped := &middleware.WrappedWriter{
			ResponseWriter: recorder,
			StatusCode:     http.StatusOK,
		}

		wrapped.WriteHeader(http.StatusNotFound)

		assert.Equal(t, http.StatusNotFound, wrapped.StatusCode, "It should return status not found")
	})

	t.Run("Forwards to responsewriter", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		wrapped := &middleware.WrappedWriter{
			ResponseWriter: recorder,
			StatusCode:     http.StatusOK,
		}

		wrapped.WriteHeader(http.StatusInternalServerError)

		assert.Equal(t, http.StatusInternalServerError, recorder.Code, "It should update the recorder")
	})

	t.Run("Starts at 200 status code", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		wrapped := &middleware.WrappedWriter{
			ResponseWriter: recorder,
			StatusCode:     http.StatusOK,
		}

		assert.Equal(t, http.StatusOK, wrapped.StatusCode, "It start as status ok")
	})
}

func TestApiRequestLogging(t *testing.T) {
	t.Run("Handler is called", func(t *testing.T) {
		handlerCalled := false
		mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		logging := middleware.ApiRequestLogging(mockHandler)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		recorder := httptest.NewRecorder()

		logging.ServeHTTP(recorder, req)

		assert.True(t, handlerCalled, "handler should be called")
	})

	t.Run("Request is passed through", func(t *testing.T) {
		var receivedMethod string
		var receivedPath string

		mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		})

		logging := middleware.ApiRequestLogging(mockHandler)
		req := httptest.NewRequest(http.MethodPost, "/api/products", nil)
		recorder := httptest.NewRecorder()

		logging.ServeHTTP(recorder, req)

		assert.Equal(t, http.MethodPost, receivedMethod, "method should be passed through")
		assert.Equal(t, "/api/products", receivedPath, "path should be passed through")
	})

	t.Run("logs status code, method, endpoint, and timer", func(t *testing.T) {
		var logBuffer bytes.Buffer
		originalOutput := log.Writer()
		log.SetOutput(&logBuffer)
		defer log.SetOutput(originalOutput)

		mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		logging := middleware.ApiRequestLogging(mockHandler)
		req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
		recorder := httptest.NewRecorder()

		logging.ServeHTTP(recorder, req)

		logOutput := logBuffer.String()

		// Verify log contains all expected fields: status code, method, path, timing
		assert.Contains(t, logOutput, "404", "Log should contain status code")
		assert.Contains(t, logOutput, "GET", "Log should contain HTTP method")
		assert.Contains(t, logOutput, "/test-path", "Log should contain request path")
		assert.True(t, strings.Contains(logOutput, "ns") || strings.Contains(logOutput, "µs") || strings.Contains(logOutput, "ms") || strings.Contains(logOutput, "s"), "Log should contain timing information")
	})

}

func TestStructuredLogger(t *testing.T) {

	t.Run("prod mode set to false should enable debug level", func(t *testing.T) {
		logger := middleware.StructuredLogger(false)

		assert.True(t, logger.Enabled(context.Background(), slog.LevelDebug))
	})

	t.Run("prod mode set to true should disable debug level", func(t *testing.T) {
		logger := middleware.StructuredLogger(true)

		assert.False(t, logger.Enabled(context.Background(), slog.LevelDebug))
		assert.True(t, logger.Enabled(context.Background(), slog.LevelInfo))
	})
}
