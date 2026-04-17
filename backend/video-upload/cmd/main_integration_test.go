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

	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	url    string
	server *http.Server
	js     jetstream.JetStream
	nc     *nats.Conn
}

func setupServer(t *testing.T) *serverEnv {
	t.Helper()
	js, nc := test.SetupNats(t)
	kv := test.SetupKV(t, js)

	cfg := &Config{HTTPPort: test.FreePort(t), StorageURL: sharedStorageURL}

	url := "http://localhost:" + cfg.HTTPPort
	server := handler.StartHttpApi(test.SilentLogger(), js, kv, cfg.HTTPPort, cfg.StorageURL)
	t.Cleanup(func() {
		server.Shutdown(context.Background()) //nolint:errcheck
	})

	require.Eventually(t, func() bool {
		resp, err := http.Post(url+"/jobs/upload", "text/plain", nil)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return true
	}, 5*time.Second, 10*time.Millisecond, "server did not start in time")

	return &serverEnv{url: url, server: server, js: js, nc: nc}
}

func TestMainErrors(t *testing.T) {
	t.Run("CreateOrUpdateKeyValue fails when JetStream is not enabled", func(t *testing.T) {
		nc := test.SetupNatsNoJetStream(t)

		js, err := jetstream.New(nc)
		require.NoError(t, err)

		_, err = js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{Bucket: "job-status"})

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
			name: "POST /jobs/download is wired to the download handler",
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

func TestUploadPipeline(t *testing.T) {
	env := setupServer(t)

	t.Run("upload writes PROCESSING state to KV and publishes scene-split message", func(t *testing.T) {
		received := make(chan []byte, 1)
		sub, err := env.nc.Subscribe("jobs.video.scene-split", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		req := test.NewUploadRequest(t, env.url+"/jobs/upload", "video.mp4", []byte("data"), "720p")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var uploadResp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&uploadResp))
		require.NotEmpty(t, uploadResp.JobID)

		// Verify KV has PROCESSING state
		kv := test.SetupKV(t, env.js)
		entry, err := kv.Get(context.Background(), uploadResp.JobID)
		require.NoError(t, err)
		var status struct {
			State string `json:"state"`
		}
		require.NoError(t, json.Unmarshal(entry.Value(), &status))
		assert.Equal(t, "PROCESSING", status.State)

		// Verify NATS scene-split message was published
		select {
		case data := <-received:
			var msg handler.SceneSplitMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, uploadResp.JobID, msg.JobID)
			assert.Equal(t, "720p", msg.TargetResolution)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for NATS message")
		}
	})

	t.Run("multiple uploads each get their own PROCESSING entry in KV", func(t *testing.T) {
		kv := test.SetupKV(t, env.js)
		jobIDs := make([]string, 3)

		for i := range jobIDs {
			req := test.NewUploadRequest(t, env.url+"/jobs/upload", "video.mp4", []byte("data"), "1080p")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusCreated, resp.StatusCode)

			var uploadResp struct {
				JobID string `json:"job_id"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&uploadResp))
			jobIDs[i] = uploadResp.JobID
		}

		for _, jobID := range jobIDs {
			entry, err := kv.Get(context.Background(), jobID)
			require.NoError(t, err, "KV entry missing for job %s", jobID)
			var status struct {
				State string `json:"state"`
			}
			require.NoError(t, json.Unmarshal(entry.Value(), &status))
			assert.Equal(t, "PROCESSING", status.State)
		}
	})
}

func TestGracefulShutdown(t *testing.T) {
	t.Run("server stops accepting connections after Shutdown", func(t *testing.T) {
		env := setupServer(t)

		resp, err := http.Post(env.url+"/jobs/upload", "text/plain", nil)
		require.NoError(t, err)
		resp.Body.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, env.server.Shutdown(ctx))

		_, err = http.Post(env.url+"/jobs/upload", "text/plain", nil)
		assert.Error(t, err, "expected connection refused after server shutdown")
	})

	t.Run("NATS drain completes without error on a healthy connection", func(t *testing.T) {
		_, nc := test.SetupNats(t)

		assert.NoError(t, nc.Drain())
	})
}
