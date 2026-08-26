//go:build integration

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
	stest "splice.com/go_services/internal/shared/test"
)

var sharedStorageURL string

func TestMain(m *testing.M) {
	url, cleanup := stest.StartSeaweedFSFiler()
	sharedStorageURL = url

	code := m.Run()

	cleanup()
	os.Exit(code)
}

// The jobs stream is created by docker-compose/nats-init in a real deployment.
// A bare NATS container has JetStream but no jobs stream, so ListenJobComplete
// cannot create its consumer and startup fails at that step.
func TestRunGateway_MissingJobsStream(t *testing.T) {
	ctx := context.Background()

	container, err := natstc.Run(ctx, "nats:2.10-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	natsURL, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	cfg := &Config{
		StorageURL: sharedStorageURL,
		NatsURL:    natsURL,
		HTTPPort:   stest.FreePort(t),
	}

	quit := make(chan os.Signal)
	err = runGateway(cfg, stest.SilentLogger(), quit)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "job complete stream")
}

// Full lifecycle: runGateway serves the merged API, an upload lands in the KV,
// and closing quit unwinds cleanly. Closing the channel replaces the old
// syscall.Kill(getpid(), SIGINT), which signalled the whole test binary.
func TestRunGateway_Lifecycle(t *testing.T) {
	js, nc := stest.SetupNats(t)

	cfg := &Config{
		StorageURL: sharedStorageURL,
		NatsURL:    nc.ConnectedUrl(),
		HTTPPort:   stest.FreePort(t),
	}
	baseURL := "http://localhost:" + cfg.HTTPPort

	quit := make(chan os.Signal)
	done := make(chan error, 1)
	go func() {
		done <- runGateway(cfg, stest.SilentLogger(), quit)
	}()

	require.Eventually(t, func() bool {
		resp, err := http.Post(baseURL+"/jobs/upload", "text/plain", nil)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return true
	}, 15*time.Second, 100*time.Millisecond, "runGateway server did not start")

	t.Run("creates the job-milestones KV bucket", func(t *testing.T) {
		// Every other service connects to this bucket and exits if it is absent,
		// so the gateway creating it is load-bearing for the whole pipeline.
		kv, err := js.KeyValue(context.Background(), "job-milestones")
		require.NoError(t, err)
		assert.Equal(t, "job-milestones", kv.Bucket())
	})

	t.Run("serves the status route", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("%s/jobs/no-such-job/status", baseURL))
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("serves the upload route", func(t *testing.T) {
		// Only that the route is mounted. Upload behaviour: PROCESSING/upload in
		// the KV, the scene-split publish is covered by TestUploadPipeline in
		// internal/gateway, against this same server.
		resp, err := http.Post(baseURL+"/jobs/upload", "text/plain", nil)
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("shuts down cleanly when quit closes", func(t *testing.T) {
		close(quit)

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(15 * time.Second):
			t.Fatal("runGateway did not return after quit closed")
		}

		_, err := http.Post(baseURL+"/jobs/upload", "text/plain", nil)
		assert.Error(t, err, "expected connection refused after shutdown")
	})
}
