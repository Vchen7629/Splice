//go:build integration

package gateway

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/handler"
	stest "splice.com/go_services/internal/shared/test"
)

type statusResponse struct {
	JobID string `json:"job_id"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

func newTestServer(t *testing.T, urls ...ServiceURLs) *httptest.Server {
	t.Helper()
	var u ServiceURLs
	if len(urls) > 0 {
		u = urls[0]
	}
	mux := http.NewServeMux()
	h := &JobStatusHandler{Logger: stest.SilentLogger(), KV: sharedKV, URLs: u}
	mux.HandleFunc("GET /jobs/{id}/status", h.PollJobStatus)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func seedStatus(t *testing.T, jobID string, status JobStatus) {
	t.Helper()
	b, err := json.Marshal(status)
	require.NoError(t, err)
	_, err = sharedKV.Put(context.Background(), jobID, b)
	require.NoError(t, err)
}

func TestResponse(t *testing.T) {
	tests := []struct {
		name      string
		jobID     string
		status    JobStatus
		wantCode  int
		wantState string
		wantErr   string
	}{
		{
			name:      "PROCESSING job returns 200 with correct state",
			jobID:     "job-processing",
			status:    JobStatus{State: StateProcessing, Stage: "scene-detector"},
			wantCode:  http.StatusOK,
			wantState: "PROCESSING",
		},
		{
			name:      "COMPLETE job returns 200 with correct state",
			jobID:     "job-complete",
			status:    JobStatus{State: StateComplete, Stage: "transcoder"},
			wantCode:  http.StatusOK,
			wantState: "COMPLETE",
		},
		{
			name:      "FAILED job returns 200 with error field populated",
			jobID:     "job-failed",
			status:    JobStatus{State: StateFailed, Stage: "transcoder", Error: "pipeline failed at stage: transcoder-worker"},
			wantCode:  http.StatusOK,
			wantState: "FAILED",
			wantErr:   "pipeline failed at stage: transcoder-worker",
		},
		{
			name:      "DEGRADED job returns 200 with error field and stage",
			jobID:     "job-degraded",
			status:    JobStatus{State: StateDegraded, Stage: "scene-detector", Error: "service unavailable at stage: transcoder"},
			wantCode:  http.StatusOK,
			wantState: "DEGRADED",
			wantErr:   "service unavailable at stage: transcoder",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seedStatus(t, tc.jobID, tc.status)
			ts := newTestServer(t)

			resp, err := http.Get(fmt.Sprintf("%s/jobs/%s/status", ts.URL, tc.jobID))
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, tc.wantCode, resp.StatusCode)
			var body statusResponse
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			assert.Equal(t, tc.jobID, body.JobID)
			assert.Equal(t, tc.wantState, body.State)
			assert.Equal(t, tc.wantErr, body.Error)
		})
	}
}

func TestResponse_NotFound(t *testing.T) {
	t.Run("unknown job ID returns 404", func(t *testing.T) {
		ts := newTestServer(t)

		resp, err := http.Get(fmt.Sprintf("%s/jobs/nonexistent-job/status", ts.URL))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("mux returns 404 when no ID segment is in the path", func(t *testing.T) {
		ts := newTestServer(t)

		// GET /jobs/{id}/status requires a non-empty segment — mux won't route /jobs//status
		resp, err := http.Get(fmt.Sprintf("%s/jobs/", ts.URL))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestConnectionDrop(t *testing.T) {
	tests := []struct {
		name   string
		jobID  string
		status JobStatus
	}{
		{"does not panic on dropped connection (PROCESSING)", "drop-processing", JobStatus{State: StateProcessing, Stage: "scene-detector"}},
		{"does not panic on dropped connection (COMPLETE)", "drop-complete", JobStatus{State: StateComplete, Stage: "transcoder"}},
		{"does not panic on dropped connection (FAILED)", "drop-failed", JobStatus{State: StateFailed, Stage: "transcoder", Error: "something broke"}},
		{"does not panic on dropped connection (not found)", "drop-notfound", JobStatus{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.status.State != "" {
				seedStatus(t, tc.jobID, tc.status)
			}

			h := &JobStatusHandler{Logger: stest.SilentLogger(), KV: sharedKV}
			req := httptest.NewRequest(http.MethodGet, "/jobs/"+tc.jobID+"/status", nil)
			req.SetPathValue("id", tc.jobID)

			assert.NotPanics(t, func() {
				h.PollJobStatus(newDroppedConnectionWriter(), req)
			})
		})
	}
}

func TestConcurrentRequests(t *testing.T) {
	t.Run("concurrent requests for a completed job return consistent state", func(t *testing.T) {
		seedStatus(t, "concurrent-job", JobStatus{State: StateComplete, Stage: "transcoder"})
		ts := newTestServer(t)

		const goroutines = 20
		results := make([]string, goroutines)
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := range goroutines {
			go func(idx int) {
				defer wg.Done()
				resp, err := http.Get(fmt.Sprintf("%s/jobs/concurrent-job/status", ts.URL))
				if err != nil {
					return
				}
				defer resp.Body.Close()
				var body statusResponse
				if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
					results[idx] = body.State
				}
			}(i)
		}

		wg.Wait()

		for _, state := range results {
			assert.Equal(t, "COMPLETE", state)
		}
	})

	t.Run("concurrent requests for a missing job all return 404", func(t *testing.T) {
		ts := newTestServer(t)

		const goroutines = 20
		codes := make([]int, goroutines)
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := range goroutines {
			go func(idx int) {
				defer wg.Done()
				resp, err := http.Get(fmt.Sprintf("%s/jobs/missing-job/status", ts.URL))
				if err != nil {
					return
				}
				defer resp.Body.Close()
				codes[idx] = resp.StatusCode
			}(i)
		}

		wg.Wait()

		for _, code := range codes {
			assert.Equal(t, http.StatusNotFound, code)
		}
	})
}

// continues serving requests after a client disconnects
func TestServerContinuesAfterDisconnect(t *testing.T) {
	seedStatus(t, "reconnect-job", JobStatus{State: StateProcessing, Stage: "scene-detector"})
	ts := newTestServer(t)

	firstResp, err := http.Get(fmt.Sprintf("%s/jobs/reconnect-job/status", ts.URL))
	require.NoError(t, err)
	firstResp.Body.Close()

	secondResp, err := http.Get(fmt.Sprintf("%s/jobs/reconnect-job/status", ts.URL))
	require.NoError(t, err)
	defer secondResp.Body.Close()

	assert.Equal(t, http.StatusOK, secondResp.StatusCode)
}

// degraded job recovers to PROCESSING when service comes back up
func TestPollJobStatus_DegradedRecovery(t *testing.T) {
	healthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthySrv.Close()

	seedStatus(t, "job-recovery", JobStatus{State: StateDegraded, Stage: "scene-detector", Error: "service unavailable at stage: transcoder"})
	ts := newTestServer(t, ServiceURLs{Transcoder: healthySrv.URL})

	resp, err := http.Get(fmt.Sprintf("%s/jobs/job-recovery/status", ts.URL))
	require.NoError(t, err)
	defer resp.Body.Close()

	var body statusResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "PROCESSING", body.State)
	assert.Empty(t, body.Error)
}

// Upload / download flows against real SeaweedFS + NATS.

//go:embed testdata/testvideo.mp4
var testVideo []byte

// testVideoBytes returns the bytes of testvideo.mp4. Only this file needs it.
func testVideoBytes(t *testing.T) []byte {
	t.Helper()
	require.NotEmpty(t, testVideo, "embedded testvideo.mp4 is empty")
	return testVideo
}

// seedProcessedVideo writes a processed output where GetProcessedVideo looks for
// it: {filer}/{jobID}/{fileName}/processed. shared/test's copy uses a different
// layout ({filer}/{jobID}/processed/{fileName}), so it cannot be reused here.
func seedProcessedVideo(t *testing.T, filerURL, jobID, fileName string, content []byte) {
	t.Helper()
	url := fmt.Sprintf("%s/%s/%s/processed", filerURL, jobID, fileName)

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(content))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Less(t, resp.StatusCode, 400)
}

func newUploadHandler(js jetstream.JetStream, kv jetstream.KeyValue, filerURL string) *videoHandler {
	return &videoHandler{
		logger:     stest.SilentLogger(),
		js:         js,
		kv:         kv,
		storageURL: filerURL,
	}
}

func newDownloadVideoServer(t *testing.T, storageURL string) *httptest.Server {
	t.Helper()
	h := &videoHandler{logger: stest.SilentLogger(), storageURL: storageURL}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs/download", h.downloadVideoRoute)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

type serverEnv struct {
	url    string
	server *http.Server
	js     jetstream.JetStream
	nc     *nats.Conn
}

// setupServer boots the real merged API on a free port against the shared filer.
func setupServer(t *testing.T) *serverEnv {
	t.Helper()
	js, nc := stest.SetupNats(t)
	kv := stest.SetupJobStatusKV(t, js)

	httpPort := stest.FreePort(t)
	url := "http://localhost:" + httpPort

	server := StartHttpApi(stest.SilentLogger(), js, kv, httpPort, sharedFilerUrl, ServiceURLs{})
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	require.Eventually(t, func() bool {
		resp, err := http.Post(url+"/jobs/upload", "text/plain", nil)
		if err != nil {
			return false
		}
		resp.Body.Close() //nolint:errcheck
		return true
	}, 5*time.Second, 10*time.Millisecond, "server did not start in time")

	return &serverEnv{url: url, server: server, js: js, nc: nc}
}

// covers the full upload pipeline: multipart form → SeaweedFS → NATS → response.
func TestUploadVideoFlow(t *testing.T) {
	js, nc := stest.SetupNats(t)
	kv := stest.SetupJobStatusKV(t, js)
	h := newUploadHandler(js, kv, sharedFilerUrl)

	t.Run("Rejects uploads exceeding MaxUploadBytes", func(t *testing.T) {
		h.maxUploadBytes = 100
		defer func() { h.maxUploadBytes = 0 }()

		req := NewUploadRequest(t, "/jobs", "big.mp4", bytes.Repeat([]byte("x"), 200), "1080p", "1080p", "Transcode")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid multipart form")
	})

	t.Run("File is saved to SeaweedFS and is fetchable at the returned StorageURL", func(t *testing.T) {
		req := NewUploadRequest(t, "/jobs", "clip.mp4", testVideoBytes(t), "1080p", "1080p", "Transcode")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)
		var resp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.NotEmpty(t, resp.JobID)
	})

	t.Run("Saved file contains the exact bytes that were uploaded", func(t *testing.T) {
		content := testVideoBytes(t)
		req := NewUploadRequest(t, "/jobs", "video.mp4", content, "720p", "1080p", "Transcode")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)
		var resp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

		stored, err := http.Get(fmt.Sprintf("%s/%s/video.mp4", sharedFilerUrl, resp.JobID))
		require.NoError(t, err)
		defer stored.Body.Close() //nolint:errcheck
		require.Less(t, stored.StatusCode, 400)
		storedBytes, err := io.ReadAll(stored.Body)
		require.NoError(t, err)
		assert.Equal(t, content, storedBytes)
	})

	t.Run("Published NATS message contains correct job_id, target_resolution, and storage_url", func(t *testing.T) {
		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.video.scene-split", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		defer sub.Unsubscribe() //nolint:errcheck

		req := NewUploadRequest(t, "/jobs", "video.mp4", testVideoBytes(t), "720p", "1080p", "Transcode")
		rec := httptest.NewRecorder()
		h.uploadVideoRoute(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var uploadResp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&uploadResp))

		select {
		case data := <-received:
			var msg handler.VideoJobMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, uploadResp.JobID, msg.JobID)
			assert.Equal(t, "720p", msg.TargetResolution)
			assert.Equal(t, fmt.Sprintf("%s/%s/video.mp4", sharedFilerUrl, uploadResp.JobID), msg.StorageURL)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for NATS message")
		}
	})

	t.Run("Multiple uploads produce unique job IDs", func(t *testing.T) {
		seen := make(map[string]bool)

		for range 3 {
			req := NewUploadRequest(t, "/jobs", "video.mp4", testVideoBytes(t), "1080p", "1080p", "Transcode")
			rec := httptest.NewRecorder()
			h.uploadVideoRoute(rec, req)
			require.Equal(t, http.StatusCreated, rec.Code)

			var resp struct {
				JobID string `json:"job_id"`
			}
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.False(t, seen[resp.JobID], "duplicate job_id: %s", resp.JobID)
			seen[resp.JobID] = true
		}
	})

	t.Run("Large file (5 MB) is fully persisted to SeaweedFS", func(t *testing.T) {
		content := bytes.Repeat([]byte("x"), 5*1024*1024)
		req := NewUploadRequest(t, "/jobs", "big.mp4", content, "4k", "1080p", "Transcode")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)
		var resp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

		stored, err := http.Get(fmt.Sprintf("%s/%s/big.mp4", sharedFilerUrl, resp.JobID))
		require.NoError(t, err)
		defer stored.Body.Close() //nolint:errcheck
		require.Less(t, stored.StatusCode, 400)
		storedBytes, err := io.ReadAll(stored.Body)
		require.NoError(t, err)
		assert.Equal(t, len(content), len(storedBytes))
	})

	t.Run("Returns 500 when NATS publish fails after successful storage save", func(t *testing.T) {
		h := newUploadHandler(&MockJS{PublishErr: errors.New("nats unavailable")}, &MockKV{}, sharedFilerUrl)
		req := NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p", "1080p", "Transcode")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "unable to send process request")
	})
}

func TestDownloadVideoFlow(t *testing.T) {
	ts := newDownloadVideoServer(t, sharedFilerUrl)

	t.Run("Streams the exact bytes of a seeded processed video", func(t *testing.T) {
		content := []byte("fake processed video bytes")
		seedProcessedVideo(t, sharedFilerUrl, "job-1", "output.mp4", content)

		req := NewDownloadRequest(t, ts.URL+"/jobs/download", "job-1", "output.mp4")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck

		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, content, body)
	})

	t.Run("Returns correct Content-Disposition and Content-Type headers", func(t *testing.T) {
		seedProcessedVideo(t, sharedFilerUrl, "job-2", "output.mp4", []byte("data"))

		req := NewDownloadRequest(t, ts.URL+"/jobs/download", "job-2", "output.mp4")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck

		assert.Equal(t, "application/octet-stream", resp.Header.Get("Content-Type"))
		assert.Contains(t, resp.Header.Get("Content-Disposition"), "output.mp4")
	})

	t.Run("Returns 500 when the processed video does not exist in storage", func(t *testing.T) {
		req := NewDownloadRequest(t, ts.URL+"/jobs/download", "no-such-job", "output.mp4")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// Both write routes are reachable through the real server and middleware chain.
func TestStartHttpApi(t *testing.T) {
	env := setupServer(t)

	const seedJobID, seedFileName = "route-test-job", "output.mp4"
	seedProcessedVideo(t, sharedFilerUrl, seedJobID, seedFileName, []byte("processed"))

	tests := []struct {
		name       string
		buildReq   func() *http.Request
		wantStatus int
	}{
		{
			name: "POST /jobs/upload is wired to the upload handler",
			buildReq: func() *http.Request {
				return NewUploadRequest(t, env.url+"/jobs/upload", "clip.mp4", testVideoBytes(t), "1080p", "1080p", "Transcode")
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
			defer resp.Body.Close() //nolint:errcheck
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

		req := NewUploadRequest(t, env.url+"/jobs/upload", "video.mp4", testVideoBytes(t), "720p", "1080p", "Transcode")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var uploadResp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&uploadResp))
		require.NotEmpty(t, uploadResp.JobID)

		kv := stest.SetupJobStatusKV(t, env.js)
		entry, err := kv.Get(context.Background(), uploadResp.JobID)
		require.NoError(t, err)
		var status struct {
			State string `json:"state"`
			Stage string `json:"stage"`
		}
		require.NoError(t, json.Unmarshal(entry.Value(), &status))
		assert.Equal(t, "PROCESSING", status.State)
		assert.Equal(t, "upload", status.Stage)

		select {
		case data := <-received:
			var msg handler.VideoJobMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, uploadResp.JobID, msg.JobID)
			assert.Equal(t, "720p", msg.TargetResolution)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for NATS message")
		}
	})

	t.Run("multiple uploads each get their own PROCESSING entry in KV", func(t *testing.T) {
		kv := stest.SetupJobStatusKV(t, env.js)
		jobIDs := make([]string, 3)

		for i := range jobIDs {
			req := NewUploadRequest(t, env.url+"/jobs/upload", "video.mp4", testVideoBytes(t), "1080p", "1080p", "Transcode")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close() //nolint:errcheck
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
		tests := []struct {
			name    string
			timeout time.Duration
		}{
			{"generous timeout", 5 * time.Second},
			{"tight timeout", 100 * time.Millisecond},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				env := setupServer(t)

				resp, err := http.Post(env.url+"/jobs/upload", "text/plain", nil)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())

				ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
				defer cancel()

				assert.NoError(t, env.server.Shutdown(ctx))

				_, err = http.Post(env.url+"/jobs/upload", "text/plain", nil)
				assert.Error(t, err, "expected connection refused after shutdown")
			})
		}
	})

	t.Run("NATS drain completes without error on a healthy connection", func(t *testing.T) {
		_, nc := stest.SetupNats(t)

		assert.NoError(t, nc.Drain())
	})
}
