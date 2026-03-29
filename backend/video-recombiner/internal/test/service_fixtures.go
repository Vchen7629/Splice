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
	for _, key := range []string{"NATS_URL", "PROD_MODE", "OUTPUT_DIR"} {
		if old, set := os.LookupEnv(key); set {
			t.Cleanup(func() { os.Setenv(key, old) })
		} else {
			t.Cleanup(func() { os.Unsetenv(key) })
		}
		os.Unsetenv(key)
	}
	path := filepath.Join("..", ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	t.Cleanup(func() { _ = os.Remove(path) })
}
