package test

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func SilentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func WriteEnvFile(t *testing.T, content string) {
	t.Helper()
	for _, key := range []string{"NATS_URL", "PROD_MODE", "BASE_STORAGE_URL"} {
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
