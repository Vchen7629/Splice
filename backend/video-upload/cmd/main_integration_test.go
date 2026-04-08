//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
	"video-upload/internal/handler"
	"video-upload/internal/test"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

var sharedStorageURL string

func TestMain(m *testing.M) {
	url, cleanup := test.StartSeaweedFSFiler()
	sharedStorageURL = url
	code := m.Run()
	cleanup()
	os.Exit(code)
}

type serverEnv struct {
	url        string
	server     *http.Server
	js         jetstream.JetStream
	storageURL string
}

// starts a NATS container, wires up the job-completion subscriber
func setupServer(t *testing.T) *serverEnv {
	t.Helper()
	js, _ := test.SetupNats(t)

	tracker, consCtx, err := handler.SubscribeJobCompletion(js, test.SilentLogger())
	require.NoError(t, err)

	cfg := &Config{HTTPPort: test.FreePort(t), StorageURL: sharedStorageURL}

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
		url:        url,
		server:     server,
		js:         js,
		storageURL: sharedStorageURL,
	}
}

func TestMainErrors(t *testing.T) {
	t.Run("no stream returns error", func(t *testing.T) {
		ctx := context.Background()

		container, err := natstc.Run(ctx, "nats:2.10-alpine")
		require.NoError(t, err)
		t.Cleanup(func() { _ = container.Terminate(ctx) })

		url, err := container.ConnectionString(ctx)
		require.NoError(t, err)

		nc, err := nats.Connect(url)
		require.NoError(t, err)
		t.Cleanup(nc.Close)

		js, err := jetstream.New(nc)
		require.NoError(t, err)

		_, _, err = handler.SubscribeJobCompletion(js, test.SilentLogger())

		assert.Error(t, err)
	})
}

func TestStartHttpApi(t *testing.T) {
	env := setupServer(t)

	const seedJobID, seedFileName = "route-test-job", "output.mp4"
	test.SeedProcessedVideo(t, sharedStorageURL, seedJobID, seedFileName, []byte("processed"))

	tests := []struct {
		name       string
		buildReq   func() *http.Request
		wantStatus int
	}{
		{
			name: "POST /jobs/upload is wired to the upload handler",
			buildReq: func() *http.Request {
				return test.NewUploadRequest(t, env.url+"/jobs/upload", "clip.mp4", []byte("data"), "1080p")
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "GET /jobs/{id}/status is wired to the job status handler",
			buildReq: func() *http.Request {
				req, _ := http.NewRequest(http.MethodGet, env.url+"/jobs/any-job/status", nil)
				return req
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "GET /jobs/download is wired to the download handler",
			buildReq: func() *http.Request {
				body := fmt.Sprintf(`{"job_id":%q,"file_name":%q}`, seedJobID, seedFileName)
				req, _ := http.NewRequest(http.MethodPost, env.url+"/jobs/download", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.DefaultClient.Do(tc.buildReq())
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
}

func TestJobCompletionFlow(t *testing.T) {
	env := setupServer(t)

	t.Run("job transitions from PROCESSING to COMPLETE after completion message is received", func(t *testing.T) {
		req := test.NewUploadRequest(t, env.url+"/jobs/upload", "video.mp4", []byte("data"), "720p")
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
