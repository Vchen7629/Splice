//go:build unit

package service_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"splice.com/go_services/internal/shared/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnect(t *testing.T) {
	t.Run("storage unreachable returns error", func(t *testing.T) {
		cfg := service.BaseConfig{BaseStorageURL: "http://localhost:1", NatsURL: "nats://localhost:1"}

		nc, js, logger, err := service.Connect("test-worker", cfg)

		require.Error(t, err)
		assert.Nil(t, nc)
		assert.Nil(t, js)
		assert.NotNil(t, logger, "logger is returned even on failure so callers can log")
	})

	t.Run("storage unhealthy status returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		nc, _, _, err := service.Connect("test-worker", service.BaseConfig{BaseStorageURL: srv.URL, NatsURL: "nats://localhost:1"})

		require.Error(t, err)
		assert.Nil(t, nc)
	})
}
