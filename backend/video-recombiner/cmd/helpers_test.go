//go:build unit || integration

package main

import (
	"os"
	"path/filepath"
	"testing"
	"video-recombiner/internal/test"

	"github.com/stretchr/testify/require"
)

func patchExit(t *testing.T) *int {
	t.Helper()
	code := -1
	osExit = func(c int) { code = c }
	t.Cleanup(func() { osExit = os.Exit })
	return &code
}

// writeEnvFile creates ../.env with the given content and removes it on cleanup.
func writeEnvFile(t *testing.T, content string) {
	t.Helper()
	for _, key := range []string{"NATS_URL", "PROD_MODE", "BASE_STORAGE_URL", "HTTP_PORT"} {
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

// okJS returns a mock JetStream that succeeds through the full consumer setup.
func okJS() *test.MockJS {
	return &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{}}}
}
