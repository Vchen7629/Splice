//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
	"video-upload/internal/handler"
	"video-upload/internal/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type serverEnv struct {
	url       string
	server    *http.Server
	js        jetstream.JetStream
	outputDir string
}

// starts a NATS container, wires up the job-completion subscriber,
// and launches the HTTP API via startHttpApi. Everything is torn down via t.Cleanup.
func setupServer(t *testing.T) *serverEnv {
	t.Helper()
	js, _ := test.SetupNats(t)

	tracker, consCtx, err := handler.SubscribeJobCompletion(js, test.SilentLogger())
	require.NoError(t, err)

	dir := t.TempDir()
	cfg := &Config{HTTPPort: test.FreePort(t), OutputDir: dir}

	url := "http://localhost:" + cfg.HTTPPort
	server := startHttpApi(test.SilentLogger(), js, tracker, cfg)
	t.Cleanup(func() {
		consCtx.Stop()
		server.Shutdown(context.Background()) //nolint:errcheck
	})

	require.Eventually(t, func() bool {
		resp, err := http.Get(url + "/jobs/_probe/status")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return true
	}, 5*time.Second, 10*time.Millisecond, "server did not start in time")

	return &serverEnv{
		url:       url,
		server:    server,
		js:        js,
		outputDir: dir,
	}
}

func TestSetupApiRoutes(t *testing.T) {
	env := setupServer(t)

	t.Run("POST /jobs is wired to the upload handler", func(t *testing.T) {
		req := test.NewUploadRequest(t, env.url+"/jobs", "clip.mp4", []byte("data"), "1080p")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("GET /jobs/{id}/status is wired to the job status handler", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("%s/jobs/any-job/status", env.url))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("GET /jobs/{id}/download is wired to the download handler", func(t *testing.T) {
		jobID := "seed-job"
		path := filepath.Join(env.outputDir, "jobs", jobID, "output.mp4")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte("data"), 0644))

		resp, err := http.Get(fmt.Sprintf("%s/jobs/%s/download", env.url, jobID))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestJobCompletionFlow(t *testing.T) {
	env := setupServer(t)

	t.Run("job transitions from PROCESSING to COMPLETE after completion message is received", func(t *testing.T) {
		req := test.NewUploadRequest(t, env.url+"/jobs", "video.mp4", []byte("data"), "720p")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		var upload struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&upload))
		resp.Body.Close()

		test.PublishJobComplete(t, env.js, upload.JobID)

		assert.Eventually(t, func() bool {
			r, err := http.Get(fmt.Sprintf("%s/jobs/%s/status", env.url, upload.JobID))
			if err != nil {
				return false
			}
			defer r.Body.Close()
			var body struct {
				State string `json:"state"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				return false
			}
			return body.State == "COMPLETE"
		}, 5*time.Second, 100*time.Millisecond)
	})
}

func TestGracefulShutdown(t *testing.T) {
	t.Run("server stops accepting connections after Shutdown", func(t *testing.T) {
		env := setupServer(t)

		resp, err := http.Get(fmt.Sprintf("%s/jobs/any/status", env.url))
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, env.server.Shutdown(ctx))

		_, err = http.Get(fmt.Sprintf("%s/jobs/any/status", env.url))
		assert.Error(t, err, "expected connection refused after server shutdown")
	})

	t.Run("NATS drain completes without error on a healthy connection", func(t *testing.T) {
		_, nc := test.SetupNats(t)

		assert.NoError(t, nc.Drain())
	})
}
