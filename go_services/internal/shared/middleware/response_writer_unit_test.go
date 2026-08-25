//go:build unit

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteHeader(t *testing.T) {
	t.Run("Captures status code properly", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		wrapped := &wrappedWriter{
			ResponseWriter: recorder,
			StatusCode:     http.StatusOK,
		}

		wrapped.WriteHeader(http.StatusNotFound)

		assert.Equal(t, http.StatusNotFound, wrapped.StatusCode, "It should return status not found")
	})

	t.Run("Forwards to responsewriter", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		wrapped := &wrappedWriter{
			ResponseWriter: recorder,
			StatusCode:     http.StatusOK,
		}

		wrapped.WriteHeader(http.StatusInternalServerError)

		assert.Equal(t, http.StatusInternalServerError, recorder.Code, "It should update the recorder")
	})

	t.Run("Starts at 200 status code", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		wrapped := &wrappedWriter{
			ResponseWriter: recorder,
			StatusCode:     http.StatusOK,
		}

		assert.Equal(t, http.StatusOK, wrapped.StatusCode, "It start as status ok")
	})
}

func TestUnwrap(t *testing.T) {
	t.Run("returns underlying ResponseWriter", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		wrapped := &wrappedWriter{ResponseWriter: recorder, StatusCode: http.StatusOK}

		assert.Same(t, http.ResponseWriter(recorder), wrapped.Unwrap())
	})

	t.Run("http.ResponseController can flush through the wrapper", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		wrapped := &wrappedWriter{ResponseWriter: recorder, StatusCode: http.StatusOK}

		err := http.NewResponseController(wrapped).Flush()

		assert.NoError(t, err)
		assert.True(t, recorder.Flushed, "Flush should have reached underlying recorder")
	})
}
