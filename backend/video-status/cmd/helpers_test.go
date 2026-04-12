//go:build unit || integration

package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"
	"video-status/internal/test"

	"github.com/stretchr/testify/require"
)

// patchOsExit replaces osExit for the duration of the test.
// The replacement captures the exit code and calls runtime.Goexit() to
// unwind the current goroutine without terminating the process.
func patchOsExit(t *testing.T) *int {
	t.Helper()
	code := new(int)
	*code = -1
	osExit = func(c int) {
		*code = c
		runtime.Goexit()
	}
	t.Cleanup(func() { osExit = os.Exit })
	return code
}

// picks a free port, starts startHttpApi, waits until the
// TCP listener is ready, and registers cleanup. Returns the base URL.
func startTestServer(t *testing.T, kv *test.MockKV) (string, *http.Server) {
	t.Helper()

	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	cfg := &Config{HTTPPort: fmt.Sprintf("%d", port)}
	server := startHttpApi(test.SilentLogger(), kv, cfg)

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	require.Eventually(t, func() bool {
		conn, err := net.Dial("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			conn.Close()
			return true
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "server did not start in time")

	return baseURL, server
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}
