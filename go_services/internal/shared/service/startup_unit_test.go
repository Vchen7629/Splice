//go:build unit

package service_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/service"
	"splice.com/go_services/internal/shared/test"

	"github.com/nats-io/nats.go/jetstream"
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

// okStart returns a StartConsumer that succeeds and exposes the consume context it hands back.
func okStart() (service.StartConsumer, *test.MockConsumeCtx) {
	ctx := &test.MockConsumeCtx{}
	return func() (jetstream.ConsumeContext, error) { return ctx, nil }, ctx
}

func failStart() (jetstream.ConsumeContext, error) { return nil, assert.AnError }

func TestRun(t *testing.T) {
	t.Run("consumer setup error returns error", func(t *testing.T) {
		nc := &test.MockDrainer{}
		quit := make(chan os.Signal, 1)

		err := service.Run(test.SilentLogger(), "0", nc, failStart, quit)

		require.ErrorIs(t, err, assert.AnError)
		assert.False(t, nc.DrainCalled, "Drain should not be called if consumer setup fails")
	})

	t.Run("blocks until quit signal", func(t *testing.T) {
		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)
		start, _ := okStart()

		go func() {
			done <- service.Run(test.SilentLogger(), "0", &test.MockDrainer{}, start, quit)
		}()

		select {
		case <-done:
			t.Fatal("Run returned before quit signal was sent")
		case <-time.After(100 * time.Millisecond):
		}

		quit <- os.Interrupt

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("Run did not return after quit signal")
		}
	})

	t.Run("stops consumer on quit", func(t *testing.T) {
		start, consCtx := okStart()
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		require.NoError(t, service.Run(test.SilentLogger(), "0", &test.MockDrainer{}, start, quit))

		assert.True(t, consCtx.Stopped)
	})

	t.Run("drains NATS on quit", func(t *testing.T) {
		nc := &test.MockDrainer{}
		start, _ := okStart()
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		require.NoError(t, service.Run(test.SilentLogger(), "0", nc, start, quit))

		assert.True(t, nc.DrainCalled)
	})

	t.Run("drain error is returned", func(t *testing.T) {
		nc := &test.MockDrainer{DrainErr: assert.AnError}
		start, _ := okStart()
		quit := make(chan os.Signal, 1)
		quit <- os.Interrupt

		err := service.Run(test.SilentLogger(), "0", nc, start, quit)

		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("server shuts down when consumer setup fails", func(t *testing.T) {
		port := test.FreePort(t)
		quit := make(chan os.Signal, 1)

		service.Run(test.SilentLogger(), port, &test.MockDrainer{}, failStart, quit) //nolint:errcheck

		// If server was properly shut down, the port should be free to bind again.
		ln, err := net.Listen("tcp", ":"+port)
		require.NoError(t, err, "port should be free after server shutdown")
		ln.Close()
	})
}
