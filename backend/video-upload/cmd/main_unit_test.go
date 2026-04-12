//go:build unit

package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"video-upload/internal/test"

	"github.com/stretchr/testify/assert"
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
func startTestServer(t *testing.T) (*http.Server, *Config) {
	t.Helper()
	fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(fakeSrv.Close)
	cfg := &Config{HTTPPort: freePort(t), StorageURL: fakeSrv.URL}
	server := startHttpApi(test.SilentLogger(), &test.MockJS{}, &test.MockKV{}, cfg)
	t.Cleanup(func() { server.Shutdown(context.Background()) }) //nolint:errcheck
	return server, cfg
}

func TestLoadConfig(t *testing.T) {
	t.Run("missing env file shouldnt return error", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join("..", ".env")); err == nil {
			t.Skip(".env already exists")
		}

		_, err := loadConfig()

		assert.NoError(t, err)
	})

	t.Run("reads all values from env file", func(t *testing.T) {
		test.WriteEnvFile(t, "NATS_URL=nats://test:9999\nPROD_MODE=true\nSTORAGE_URL=http://storage:9333\nHTTP_PORT=9090\n")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "nats://test:9999", cfg.NatsURL)
		assert.True(t, cfg.ProdMode)
		assert.Equal(t, "http://storage:9333", cfg.StorageURL)
		assert.Equal(t, "9090", cfg.HTTPPort)
	})

	t.Run("empty env file uses struct defaults", func(t *testing.T) {
		test.WriteEnvFile(t, "")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "nats://localhost:4222", cfg.NatsURL)
		assert.False(t, cfg.ProdMode)
		assert.Equal(t, "http://localhost:8888", cfg.StorageURL)
		assert.Equal(t, "8080", cfg.HTTPPort)
	})
}

func TestStartHttp(t *testing.T) {
	t.Run("returns non-nil server with address derived from config", func(t *testing.T) {
		server, cfg := startTestServer(t)

		require.NotNil(t, server)
		assert.Equal(t, ":"+cfg.HTTPPort, server.Addr)
	})

	t.Run("server handler is non-nil", func(t *testing.T) {
		server, _ := startTestServer(t)

		assert.NotNil(t, server.Handler)
	})

	t.Run("unregistered path returns 404", func(t *testing.T) {
		server, _ := startTestServer(t)

		req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
		w := httptest.NewRecorder()
		server.Handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestMainFunc(t *testing.T) {
	t.Run("exits on config load error", func(t *testing.T) {
		if os.Getenv("RUN_MAIN") == "config_error" {
			os.Chdir("/") //nolint:errcheck
			main()
			return
		}
		cmd := exec.Command(os.Args[0], "-test.run=TestMain/exits_on_config_load_error", "-test.count=1")
		cmd.Env = append(os.Environ(), "RUN_MAIN=config_error")
		err := cmd.Run()
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		assert.Equal(t, 1, exitErr.ExitCode())
	})

	t.Run("exits on NATS connect error", func(t *testing.T) {
		if os.Getenv("RUN_MAIN") == "nats_error" {
			main()
			return
		}
		test.WriteEnvFile(t, "NATS_URL=nats://localhost:1\n")
		var env []string
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "NATS_URL=") && !strings.HasPrefix(e, "PROD_MODE=") &&
				!strings.HasPrefix(e, "STORAGE_URL=") && !strings.HasPrefix(e, "HTTP_PORT=") {
				env = append(env, e)
			}
		}
		cmd := exec.Command(os.Args[0], "-test.run=TestMain/exits_on_NATS_connect_error", "-test.count=1")
		cmd.Env = append(env, "RUN_MAIN=nats_error")
		err := cmd.Run()
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		assert.Equal(t, 1, exitErr.ExitCode())
	})

	t.Run("returns 500 when KV.Put fails during upload", func(t *testing.T) {
		fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		t.Cleanup(fakeSrv.Close)
		cfg := &Config{HTTPPort: freePort(t), StorageURL: fakeSrv.URL}
		kv := &test.MockKV{PutErr: errors.New("kv unavailable")}
		server := startHttpApi(test.SilentLogger(), &test.MockJS{}, kv, cfg)
		t.Cleanup(func() { server.Shutdown(context.Background()) }) //nolint:errcheck

		req := test.NewUploadRequest(t, "/jobs/upload", "video.mp4", []byte("data"), "1080p")
		w := httptest.NewRecorder()
		server.Handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to record job status")
	})
}
