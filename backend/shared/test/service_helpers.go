//go:build unit

package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type MockConfig struct {
	NatsURL    string `envconfig:"NATS_URL" default:"nats://localhost:4222"`
	ProdMode   bool   `envconfig:"PROD_MODE" default:"false"`
	StorageURL string `envconfig:"STORAGE_URL" default:"http://localhost:8888"`
	HTTPPort   string `envconfig:"HTTP_PORT" default:"8080"`
}

func WriteEnvFile(t *testing.T, content string) {
	t.Helper()
	for _, key := range []string{"NATS_URL", "PROD_MODE", "STORAGE_URL", "HTTP_PORT"} {
		if old, set := os.LookupEnv(key); set {
			t.Cleanup(func() {
				err := os.Setenv(key, old)
				require.NoError(t, err)
			})
		} else {
			t.Cleanup(func() {
				err := os.Unsetenv(key)
				require.NoError(t, err)
			})
		}
		err := os.Unsetenv(key)
		require.NoError(t, err)
	}
	path := filepath.Join("..", ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	t.Cleanup(func() { _ = os.Remove(path) })
}