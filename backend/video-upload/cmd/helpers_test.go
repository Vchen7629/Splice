package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"video-upload/internal/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// freePort returns a port number that is not currently in use.
func freePort(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)

	l.Close()

	return port
}

// startTestServer calls startHttpApi with a free port and a temp output dir,
// registers a Cleanup to shut the server down, and returns the server and cfg.
func startTestServer(t *testing.T, kv jetstream.KeyValue) (*http.Server, *Config) {
	t.Helper()

	fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	t.Cleanup(fakeSrv.Close)

	cfg := &Config{HTTPPort: freePort(t), StorageURL: fakeSrv.URL}
	server := startHttpApi(test.SilentLogger(), &test.MockJS{}, kv, cfg)

	t.Cleanup(func() { server.Shutdown(context.Background()) }) //nolint:errcheck

	return server, cfg
}