//go:build integration

package service_test

import (
	"os"
	"syscall"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/service"
	"splice.com/go_services/internal/shared/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sharedFilerURL string

func TestMain(m *testing.M) {
	filerURL, cleanup := test.StartSeaweedFSFiler()
	sharedFilerURL = filerURL

	code := m.Run()

	cleanup()
	os.Exit(code)
}

func TestConnectI(t *testing.T) {
	t.Run("nats unreachable returns error", func(t *testing.T) {
		cfg := service.BaseConfig{BaseStorageURL: sharedFilerURL, NatsURL: "nats://localhost:1"}

		nc, js, _, err := service.Connect("test-worker", cfg)

		require.Error(t, err)
		assert.Nil(t, nc)
		assert.Nil(t, js)
	})

	t.Run("healthy storage and nats returns connections", func(t *testing.T) {
		_, natsConn := test.SetupNats(t)
		cfg := service.BaseConfig{BaseStorageURL: sharedFilerURL, NatsURL: natsConn.ConnectedUrl()}

		nc, js, logger, err := service.Connect("test-worker", cfg)

		require.NoError(t, err)
		t.Cleanup(nc.Close)
		assert.NotNil(t, js)
		assert.NotNil(t, logger)
		assert.True(t, nc.IsConnected())
	})
}

func TestRunI(t *testing.T) {
	t.Run("quit signal stops consumer, drains real nats and exits cleanly", func(t *testing.T) {
		_, nc := test.SetupNats(t)
		consCtx := &test.MockConsumeCtx{}
		start := func() (jetstream.ConsumeContext, error) { return consCtx, nil }
		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)

		go func() {
			done <- service.Run(test.SilentLogger(), "0", nc, start, quit)
		}()

		time.Sleep(200 * time.Millisecond)
		quit <- syscall.SIGTERM

		select {
		case err := <-done:
			assert.NoError(t, err)
			assert.True(t, consCtx.Stopped)
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not exit after quit signal")
		}
	})
}
