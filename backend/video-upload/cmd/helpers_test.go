//go:build unit || integration

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// patchOsExit replaces osExit with a recorder and restores it after the test.
func patchOsExit(t *testing.T) *int {
	t.Helper()
	code := new(int)
	*code = -1
	osExit = func(c int) { *code = c }
	t.Cleanup(func() { osExit = os.Exit })
	return code
}

// patchNatsConnect replaces natsConnect with a stub that returns an error.
func patchNatsConnect(t *testing.T, err error) {
	t.Helper()
	natsConnect = func(_ string, _ ...nats.Option) (*nats.Conn, error) { return nil, err }
	t.Cleanup(func() { natsConnect = nats.Connect })
}

// fakeStorageServer starts an httptest.Server that accepts any request and returns 200.
// Used to make storage.CheckHealth succeed so tests can reach later startup steps.
func fakeStorageServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// writeEnvFile creates ../.env with the given content and unsets the relevant env vars for the test.
func writeEnvFile(t *testing.T, content string) {
	t.Helper()
	for _, key := range []string{"NATS_URL", "PROD_MODE", "STORAGE_URL", "HTTP_PORT"} {
		if old, set := os.LookupEnv(key); set {
			t.Cleanup(func() { os.Setenv(key, old) }) //nolint:errcheck
		} else {
			t.Cleanup(func() { os.Unsetenv(key) }) //nolint:errcheck
		}
		os.Unsetenv(key) //nolint:errcheck
	}
	require.NoError(t, os.WriteFile("../.env", []byte(content), 0600))
	t.Cleanup(func() { _ = os.Remove("../.env") })
}
