//go:build integration

package service_test

import (
	"os"
	"testing"

	"splice.com/go_services/internal/shared/service"
	"splice.com/go_services/internal/shared/test"

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
