//go:build unit

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	stest "splice.com/go_services/internal/shared/test"
)

// patchNatsConnect replaces natsConnect with a stub that returns an error.
func patchNatsConnect(t *testing.T, err error) {
	t.Helper()
	natsConnect = func(_ string, _ ...nats.Option) (*nats.Conn, error) { return nil, err }
	t.Cleanup(func() { natsConnect = nats.Connect })
}

// fakeStorageServer starts a server that accepts any request and returns 200, so
// storage.CheckHealth succeeds and the test reaches the later startup steps.
func fakeStorageServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// runGateway returns a wrapped error for every startup failure, so each case
// asserts on which step failed rather than just on an exit code.
func TestRunGateway_StartupFailures(t *testing.T) {
	tests := []struct {
		name            string
		cfg             func(t *testing.T) *Config
		setup           func(t *testing.T)
		wantErrContains string
	}{
		{
			name: "returns error when storage is unreachable",
			cfg: func(t *testing.T) *Config {
				return &Config{StorageURL: "http://localhost:1", NatsURL: "nats://localhost:1"}
			},
			wantErrContains: "storage seedweedfs unreachable",
		},
		{
			name: "returns error when nats is unreachable",
			cfg: func(t *testing.T) *Config {
				return &Config{StorageURL: fakeStorageServer(t), NatsURL: "nats://localhost:1"}
			},
			setup: func(t *testing.T) {
				patchNatsConnect(t, errors.New("nats unreachable"))
			},
			wantErrContains: "unable to connect to nats",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t)
			}

			quit := make(chan os.Signal)
			err := runGateway(tc.cfg(t), stest.SilentLogger(), quit)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrContains)
		})
	}
}

// A real dial failure (rather than a patched natsConnect) reaches the same branch.
func TestRunGateway_RealNatsDialFailure(t *testing.T) {
	cfg := &Config{StorageURL: fakeStorageServer(t), NatsURL: "nats://localhost:1"}

	quit := make(chan os.Signal)
	err := runGateway(cfg, stest.SilentLogger(), quit)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to connect to nats")
}
