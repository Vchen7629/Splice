//go:build unit

package service

import (
	"os"
	"path/filepath"
	"shared/test"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("missing env file shouldnt return error", func(t *testing.T) {
		_, err := os.Stat(filepath.Join("..", ".env"))
		if err == nil {
			t.Skip(".env already exists")
		}

		_, err = LoadConfig[test.MockConfig]()

		assert.NoError(t, err)
	})

	t.Run("reads all values from env file", func(t *testing.T) {
		test.WriteEnvFile(t, "NATS_URL=nats://test:9999\nPROD_MODE=true\nSTORAGE_URL=http://storage:9333\nHTTP_PORT=9090\n")

		cfg, err := LoadConfig[test.MockConfig]()

		require.NoError(t, err)
		assert.Equal(t, "nats://test:9999", cfg.NatsURL)
		assert.True(t, cfg.ProdMode)
		assert.Equal(t, "http://storage:9333", cfg.StorageURL)
		assert.Equal(t, "9090", cfg.HTTPPort)
	})

	t.Run("empty env file uses struct defaults", func(t *testing.T) {
		test.WriteEnvFile(t, "")

		cfg, err := LoadConfig[test.MockConfig]()

		require.NoError(t, err)
		assert.Equal(t, "nats://localhost:4222", cfg.NatsURL)
		assert.False(t, cfg.ProdMode)
		assert.Equal(t, "http://localhost:8888", cfg.StorageURL)
		assert.Equal(t, "8080", cfg.HTTPPort)
	})
}
