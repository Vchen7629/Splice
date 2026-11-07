//go:build integration

package gateway

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/pipeline"
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
	jh := &JobStatusHandler{Logger: stest.SilentLogger(), NC: sharedNC, KV: sharedKV, URLs: u, HealthClient: &http.Client{Timeout: 3 * time.Second}}
	ch := &cancelHandler{logger: stest.SilentLogger(), kv: sharedKV}
	mux.HandleFunc("GET /jobs/{id}/status", jh.PollJobStatus)
	mux.HandleFunc("GET /jobs/{id}/events", jh.JobEvents)
	mux.HandleFunc("DELETE /jobs/{id}", ch.cancelProcessingRoute)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func seedStatus(t *testing.T, jobID string, status pipeline.JobStatus) {
	t.Helper()
	b, err := json.Marshal(status)
	require.NoError(t, err)
	_, err = sharedKV.Put(context.Background(), jobID, b)
	require.NoError(t, err)
}

func TestHTTPResponse(t *testing.T) {
	tests := []struct {
		name      string
		jobID     string
		status    pipeline.JobStatus
		wantCode  int
		wantState string
		wantErr   string
	}{
		{
			name:      "PROCESSING job returns 200 with correct state",
			jobID:     uuid.New().String(),
			status:    pipeline.JobStatus{State: pipeline.StateProcessing, Stage: "scene-detector"},
			wantCode:  http.StatusOK,
			wantState: "PROCESSING",
		},
		{
			name:      "COMPLETE job returns 200 with correct state",
			jobID:     uuid.New().String(),
			status:    pipeline.JobStatus{State: pipeline.StateComplete, Stage: "chunk-worker"},
			wantCode:  http.StatusOK,
			wantState: "COMPLETE",
		},
		{
			name:      "FAILED job returns 200 with error field populated",
			jobID:     uuid.New().String(),
			status:    pipeline.JobStatus{State: pipeline.StateFailed, Stage: "chunk-worker", Error: "pipeline failed at stage: chunk-worker"},
			wantCode:  http.StatusOK,
			wantState: "FAILED",
			wantErr:   "pipeline failed at stage: chunk-worker",
		},
		{
			name:      "DEGRADED job returns 200 with error field and stage",
			jobID:     uuid.New().String(),
			status:    pipeline.JobStatus{State: pipeline.StateDegraded, Stage: "scene-detector", Error: "service unavailable at stage: chunk-worker"},
			wantCode:  http.StatusOK,
			wantState: "DEGRADED",
			wantErr:   "service unavailable at stage: chunk-worker",
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

	t.Run("unknown job ID returns 404", func(t *testing.T) {
		ts := newTestServer(t)

		resp, err := http.Get(fmt.Sprintf("%s/jobs/%s/status", ts.URL, uuid.New().String()))
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
		status pipeline.JobStatus
	}{
		{"does not panic on dropped connection (PROCESSING)", uuid.New().String(), pipeline.JobStatus{State: pipeline.StateProcessing, Stage: "scene-detector"}},
		{"does not panic on dropped connection (COMPLETE)", uuid.New().String(), pipeline.JobStatus{State: pipeline.StateComplete, Stage: "chunk-worker"}},
		{"does not panic on dropped connection (FAILED)", uuid.New().String(), pipeline.JobStatus{State: pipeline.StateFailed, Stage: "chunk-worker", Error: "something broke"}},
		{"does not panic on dropped connection (not found)", uuid.New().String(), pipeline.JobStatus{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.status.State != "" {
				seedStatus(t, tc.jobID, tc.status)
			}

			h := &JobStatusHandler{Logger: stest.SilentLogger(), KV: sharedKV, HealthClient: &http.Client{Timeout: 3 * time.Second}}
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
		jobID := uuid.New().String()
		seedStatus(t, jobID, pipeline.JobStatus{State: pipeline.StateComplete, Stage: "chunk-worker"})
		ts := newTestServer(t)

		const goroutines = 20
		results := make([]string, goroutines)
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := range goroutines {
			go func(idx int) {
				defer wg.Done()
				resp, err := http.Get(fmt.Sprintf("%s/jobs/%s/status", ts.URL, jobID))
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
				resp, err := http.Get(fmt.Sprintf("%s/jobs/%s/status", ts.URL, uuid.New().String()))
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
	jobID := uuid.New().String()
	seedStatus(t, jobID, pipeline.JobStatus{State: pipeline.StateProcessing, Stage: "scene-detector"})
	ts := newTestServer(t)

	firstResp, err := http.Get(fmt.Sprintf("%s/jobs/%s/status", ts.URL, jobID))
	require.NoError(t, err)
	firstResp.Body.Close()

	secondResp, err := http.Get(fmt.Sprintf("%s/jobs/%s/status", ts.URL, jobID))
	require.NoError(t, err)
	defer secondResp.Body.Close()

	assert.Equal(t, http.StatusOK, secondResp.StatusCode)
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
func seedProcessedVideo(t *testing.T, filerURL, fileName string, jobID uuid.UUID, content []byte) {
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
	kv := stest.SetupJobMilestoneKV(t, js)

	httpPort := stest.FreePort(t)
	url := "http://localhost:" + httpPort

	server := StartHttpApi(stest.SilentLogger(), nil, js, kv, Config{
		httpPort, 10, sharedFilerUrl, ServiceURLs{},
	})
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

// Both write routes are reachable through the real server and middleware chain.
func TestStartHttpApi(t *testing.T) {
	env := setupServer(t)

	seedJobID := uuid.New()
	const seedFileName = "output.mp4"
	seedProcessedVideo(t, sharedFilerUrl, seedFileName, seedJobID, []byte("processed"))

	tests := []struct {
		name       string
		buildReq   func() *http.Request
		wantStatus int
	}{
		{
			name: "POST /jobs/upload is wired to the upload handler",
			buildReq: func() *http.Request {
				return NewUploadRequest(t, env.url+"/jobs/upload", "clip.mp4", testVideoBytes(t), "1080p", "transcode")
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

				// close spare keep-alive connection it never sent a request on that cause flakyness
				http.DefaultClient.CloseIdleConnections()

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

func TestCancelRouteI(t *testing.T) {
	t.Run("happy path for processing", func(t *testing.T) {
		jobID := uuid.New().String()
		seedStatus(t, jobID, pipeline.JobStatus{State: pipeline.StateProcessing, Stage: "scene-detector"})
		ts := newTestServer(t)

		req, err := http.NewRequest(http.MethodDelete, ts.URL+"/jobs/"+jobID, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var got jobStatusResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Equal(t, jobID, got.JobID)
		assert.Equal(t, "CANCELLED", string(got.State))

		entry, err := sharedKV.Get(context.Background(), jobID)
		require.NoError(t, err)
		var stored pipeline.JobStatus
		require.NoError(t, json.Unmarshal(entry.Value(), &stored))
		assert.Equal(t, pipeline.StateCancelled, stored.State)
	})

	t.Run("repeated cancels (3 sequential delete) all return 200 CANCELLED and only one KV revision bump", func(t *testing.T) {
		jobID := uuid.New().String()
		seedStatus(t, jobID, pipeline.JobStatus{State: pipeline.StateProcessing, Stage: "scene-detector"})
		ts := newTestServer(t)

		before, err := sharedKV.Get(context.Background(), jobID)
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			req, err := http.NewRequest(http.MethodDelete, ts.URL+"/jobs/"+jobID, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			var got jobStatusResponse
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
			resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "CANCELLED", string(got.State))
		}

		entry, err := sharedKV.Get(context.Background(), jobID)
		require.NoError(t, err)
		assert.Equal(t, before.Revision()+1, entry.Revision(), "only the first cancel should CAS-write; the two repeats must no-op")
	})

	t.Run("cancel on COMPLETED job is no-op", func(t *testing.T) {
		jobID := uuid.New().String()
		seedStatus(t, jobID, pipeline.JobStatus{State: pipeline.StateComplete, Stage: "scene-detector"})
		ts := newTestServer(t)

		before, err := sharedKV.Get(context.Background(), jobID)
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodDelete, ts.URL+"/jobs/"+jobID, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var got jobStatusResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Equal(t, "COMPLETE", string(got.State))

		after, err := sharedKV.Get(context.Background(), jobID)
		require.NoError(t, err)
		assert.Equal(t, before.Revision(), after.Revision())
	})
}
