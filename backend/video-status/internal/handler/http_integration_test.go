//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"video-status/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	h := &JobStatusHandler{Logger: test.SilentLogger(), KV: sharedKV, URLs: u}
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

			h := &JobStatusHandler{Logger: test.SilentLogger(), KV: sharedKV}
			req := httptest.NewRequest(http.MethodGet, "/jobs/"+tc.jobID+"/status", nil)
			req.SetPathValue("id", tc.jobID)

			assert.NotPanics(t, func() {
				h.PollJobStatus(test.NewDroppedConnectionWriter(), req)
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
