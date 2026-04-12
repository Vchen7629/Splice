//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"video-status/internal/handler"
	"video-status/internal/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statusResponse struct {
	JobID string `json:"job_id"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

func newTestServer(t *testing.T, kv jetstream.KeyValue) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	h := &handler.JobStatusHandler{Logger: test.SilentLogger(), KV: kv}
	mux.HandleFunc("GET /jobs/{id}/status", h.PollJobStatus)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func seedStatus(t *testing.T, kv jetstream.KeyValue, jobID string, status handler.JobStatus) {
	t.Helper()
	b, err := json.Marshal(status)
	require.NoError(t, err)
	_, err = kv.Put(context.Background(), jobID, b)
	require.NoError(t, err)
}

func TestResponse(t *testing.T) {
	tests := []struct {
		name      string
		jobID     string
		status    handler.JobStatus
		wantCode  int
		wantState string
		wantErr   string
	}{
		{
			name:      "PROCESSING job returns 200 with correct state",
			jobID:     "job-processing",
			status:    handler.JobStatus{State: handler.StateProcessing},
			wantCode:  http.StatusOK,
			wantState: "PROCESSING",
		},
		{
			name:      "COMPLETE job returns 200 with correct state",
			jobID:     "job-complete",
			status:    handler.JobStatus{State: handler.StateComplete},
			wantCode:  http.StatusOK,
			wantState: "COMPLETE",
		},
		{
			name:      "FAILED job returns 200 with error field populated",
			jobID:     "job-failed",
			status:    handler.JobStatus{State: handler.StateFailed, Error: "pipeline failed at stage: transcoder-worker"},
			wantCode:  http.StatusOK,
			wantState: "FAILED",
			wantErr:   "pipeline failed at stage: transcoder-worker",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			js, _ := test.SetupNats(t)
			kv := test.SetupKV(t, js)
			seedStatus(t, kv, tc.jobID, tc.status)
			ts := newTestServer(t, kv)

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
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)
		ts := newTestServer(t, kv)

		resp, err := http.Get(fmt.Sprintf("%s/jobs/nonexistent-job/status", ts.URL))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("mux returns 404 when no ID segment is in the path", func(t *testing.T) {
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)
		ts := newTestServer(t, kv)

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
		status handler.JobStatus
	}{
		{"does not panic on dropped connection (PROCESSING)", handler.JobStatus{State: handler.StateProcessing}},
		{"does not panic on dropped connection (COMPLETE)", handler.JobStatus{State: handler.StateComplete}},
		{"does not panic on dropped connection (FAILED)", handler.JobStatus{State: handler.StateFailed, Error: "something broke"}},
		{"does not panic on dropped connection (not found)", handler.JobStatus{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			js, _ := test.SetupNats(t)
			kv := test.SetupKV(t, js)

			if tc.status.State != "" {
				seedStatus(t, kv, "job-1", tc.status)
			}

			h := &handler.JobStatusHandler{Logger: test.SilentLogger(), KV: kv}
			req := httptest.NewRequest(http.MethodGet, "/jobs/job-1/status", nil)
			req.SetPathValue("id", "job-1")

			assert.NotPanics(t, func() {
				h.PollJobStatus(test.NewDroppedConnectionWriter(), req)
			})
		})
	}
}

func TestConcurrentRequests(t *testing.T) {
	t.Run("concurrent requests for a completed job return consistent state", func(t *testing.T) {
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)
		seedStatus(t, kv, "concurrent-job", handler.JobStatus{State: handler.StateComplete})
		ts := newTestServer(t, kv)

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
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)
		ts := newTestServer(t, kv)

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

func TestServerContinuesAfterDisconnect(t *testing.T) {
	t.Run("server continues serving requests after a client disconnects", func(t *testing.T) {
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)
		seedStatus(t, kv, "job-1", handler.JobStatus{State: handler.StateProcessing})
		ts := newTestServer(t, kv)

		firstResp, err := http.Get(fmt.Sprintf("%s/jobs/job-1/status", ts.URL))
		require.NoError(t, err)
		firstResp.Body.Close()

		secondResp, err := http.Get(fmt.Sprintf("%s/jobs/job-1/status", ts.URL))
		require.NoError(t, err)
		defer secondResp.Body.Close()

		assert.Equal(t, http.StatusOK, secondResp.StatusCode)
	})
}
