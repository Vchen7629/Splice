//go:build unit

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"video-upload/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("missing env file shouldnt return error", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join("..", ".env")); err == nil {
			t.Skip(".env already exists")
		}

		_, err := loadConfig()

		assert.NoError(t, err)
	})

	t.Run("reads all values from env file", func(t *testing.T) {
		test.WriteEnvFile(t, "NATS_URL=nats://test:9999\nPROD_MODE=true\nSTORAGE_URL=http://storage:9333\nHTTP_PORT=9090\n")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "nats://test:9999", cfg.NatsURL)
		assert.True(t, cfg.ProdMode)
		assert.Equal(t, "http://storage:9333", cfg.StorageURL)
		assert.Equal(t, "9090", cfg.HTTPPort)
	})

	t.Run("empty env file uses struct defaults", func(t *testing.T) {
		test.WriteEnvFile(t, "")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "nats://localhost:4222", cfg.NatsURL)
		assert.False(t, cfg.ProdMode)
		assert.Equal(t, "http://localhost:8888", cfg.StorageURL)
		assert.Equal(t, "8080", cfg.HTTPPort)
	})
}

func TestMainFunc(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{
			name: "exits when storage is unreachable",
			setup: func(t *testing.T) {
				writeEnvFile(t, "STORAGE_URL=http://localhost:1\n")
			},
		},
		{
			name: "exits when nats is unreachable",
			setup: func(t *testing.T) {
				writeEnvFile(t, "STORAGE_URL="+fakeStorageServer(t)+"\nNATS_URL=nats://localhost:1\n")
				patchNatsConnect(t, errors.New("nats unreachable"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := patchOsExit(t)
			tc.setup(t)

			main()

			assert.Equal(t, 1, *code)
		})
	}
}
