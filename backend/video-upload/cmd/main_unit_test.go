//go:build unit

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	t.Run("exits on NATS connect error", func(t *testing.T) {
		if os.Getenv("RUN_MAIN") == "nats_error" {
			main()
			return
		}

		test.WriteEnvFile(t, "NATS_URL=nats://localhost:1\n")

		var env []string
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "NATS_URL=") && !strings.HasPrefix(e, "PROD_MODE=") &&
				!strings.HasPrefix(e, "STORAGE_URL=") && !strings.HasPrefix(e, "HTTP_PORT=") {
				env = append(env, e)
			}
		}

		cmd := exec.Command(os.Args[0], "-test.run=TestMain/exits_on_NATS_connect_error", "-test.count=1")
		cmd.Env = append(env, "RUN_MAIN=nats_error")
		err := cmd.Run()

		var exitErr *exec.ExitError

		require.ErrorAs(t, err, &exitErr)
		assert.Equal(t, 1, exitErr.ExitCode())
	})
}
