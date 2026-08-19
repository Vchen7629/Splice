//go:build unit || integration

package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// PatchExit points a cmd's osExit seam at a recorder for the duration of the
// test and restores it afterwards. The returned pointer holds the last code
// passed to the seam, or -1 if it was never called.
//
//	code := test.PatchExit(t, &osExit)
//
// Note the replacement returns normally rather than calling runtime.Goexit, so
// main() must `return` after every osExit(1) for the test to observe the exit.
func PatchExit(t *testing.T, seam *func(int)) *int {
	t.Helper()
	code := new(int)
	*code = -1

	original := *seam
	*seam = func(c int) { *code = c }
	t.Cleanup(func() { *seam = original })

	return code
}

// configEnvKeys is the union of every env var read by any service's Config.
// Clearing all of them regardless of caller keeps one service's leftovers from
// bleeding into another's test — the per-service copies this replaced each
// cleared only their own subset.
var configEnvKeys = []string{
	"NATS_URL",
	"PROD_MODE",
	"STORAGE_URL",
	"BASE_STORAGE_URL",
	"HTTP_PORT",
	"SCENE_DETECTOR_URL",
	"TRANSCODER_URL",
	"RECOMBINER_URL",
}

// WriteEnvFile creates ../.env with the given content, clears the config env
// vars so they cannot mask it, and restores both on cleanup. The path is
// relative to the package under test, matching service.LoadConfig's godotenv.Load("../.env").
func WriteEnvFile(t *testing.T, content string) {
	t.Helper()

	for _, key := range configEnvKeys {
		if old, set := os.LookupEnv(key); set {
			t.Cleanup(func() { require.NoError(t, os.Setenv(key, old)) })
		} else {
			t.Cleanup(func() { require.NoError(t, os.Unsetenv(key)) })
		}
		require.NoError(t, os.Unsetenv(key))
	}

	path := filepath.Join("..", ".env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	t.Cleanup(func() { _ = os.Remove(path) })
}
