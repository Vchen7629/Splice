//go:build unit || integration

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func patchOsExit(t *testing.T) *int {
	t.Helper()
	code := new(int)
	*code = -1
	osExit = func(c int) {
		*code = c
	}
	t.Cleanup(func() { osExit = os.Exit })
	return code
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
