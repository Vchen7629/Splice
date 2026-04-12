//go:build unit

package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"video-status/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHandler(kv *test.MockKV) *JobStatusHandler {
	return &JobStatusHandler{Logger: test.SilentLogger(), KV: kv}
}

func mustMarshalStatus(t *testing.T, status JobStatus) []byte {
	t.Helper()
	b, err := json.Marshal(status)
	require.NoError(t, err)
	return b
}

func TestPollJobStatus_BadRequest(t *testing.T) {
	h := newHandler(test.NewMockKV())
	req := httptest.NewRequest(http.MethodGet, "/jobs//status", nil)
	// path value is empty string — simulates missing segment
	req.SetPathValue("id", "")
	rec := httptest.NewRecorder()

	h.PollJobStatus(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing job_id")

	var resp jobStatusResponse
	assert.Error(t, json.Unmarshal(rec.Body.Bytes(), &resp), "error response should not be valid JSON")
}

func TestPollJobStatus_KVErrors(t *testing.T) {
	kvErr := errors.New("kv unavailable")

	tests := []struct {
		name       string
		kv         *test.MockKV
		wantStatus int
		wantBody   string
	}{
		{
			name:       "key not found returns 404",
			kv:         test.NewMockKV(),
			wantStatus: http.StatusNotFound,
			wantBody:   "job not found",
		},
		{
			name: "generic KV error returns 500",
			kv: func() *test.MockKV {
				m := test.NewMockKV()
				m.GetErr = kvErr
				return m
			}(),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to get job status",
		},
		{
			name: "malformed KV value returns 500",
			kv: func() *test.MockKV {
				m := test.NewMockKV()
				m.Seed("job-1", []byte("not valid json{{"))
				return m
			}(),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to parse job status",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandler(tc.kv)
			req := httptest.NewRequest(http.MethodGet, "/jobs/job-1/status", nil)
			req.SetPathValue("id", "job-1")
			rec := httptest.NewRecorder()

			h.PollJobStatus(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantBody)

			var resp jobStatusResponse
			assert.Error(t, json.Unmarshal(rec.Body.Bytes(), &resp), "error response should not be valid JSON")
		})
	}
}

func TestPollJobStatus_States(t *testing.T) {
	tests := []struct {
		name       string
		status     JobStatus
		wantState  JobState
		wantErrMsg string
	}{
		{
			name:      "PROCESSING state",
			status:    JobStatus{State: StateProcessing},
			wantState: StateProcessing,
		},
		{
			name:      "COMPLETE state",
			status:    JobStatus{State: StateComplete},
			wantState: StateComplete,
		},
		{
			name:       "FAILED state includes error message",
			status:     JobStatus{State: StateFailed, Error: "pipeline failed at stage: transcoder-worker"},
			wantState:  StateFailed,
			wantErrMsg: "pipeline failed at stage: transcoder-worker",
		},
		{
			name:      "FAILED with empty error field",
			status:    JobStatus{State: StateFailed},
			wantState: StateFailed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kv := test.NewMockKV()
			kv.Seed("job-1", mustMarshalStatus(t, tc.status))
			h := newHandler(kv)

			req := httptest.NewRequest(http.MethodGet, "/jobs/job-1/status", nil)
			req.SetPathValue("id", "job-1")
			rec := httptest.NewRecorder()

			h.PollJobStatus(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			var resp jobStatusResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, tc.wantState, resp.State)
			assert.Equal(t, tc.wantErrMsg, resp.Error)
		})
	}
}

func TestPollJobStatus_ResponseShape(t *testing.T) {
	tests := []struct {
		name  string
		jobID string
	}{
		{"echoes job_id in response", "my-specific-job"},
		{"echoes different job_id", "another-job-456"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kv := test.NewMockKV()
			kv.Seed(tc.jobID, mustMarshalStatus(t, JobStatus{State: StateProcessing}))
			h := newHandler(kv)

			req := httptest.NewRequest(http.MethodGet, "/jobs/"+tc.jobID+"/status", nil)
			req.SetPathValue("id", tc.jobID)
			rec := httptest.NewRecorder()

			h.PollJobStatus(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			var resp jobStatusResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, tc.jobID, resp.JobID)
			assert.NotEmpty(t, resp.State)
		})
	}
}

func TestPollJobStatus_DroppedConnection(t *testing.T) {
	tests := []struct {
		name   string
		status JobStatus
	}{
		{"does not panic on dropped connection (PROCESSING)", JobStatus{State: StateProcessing}},
		{"does not panic on dropped connection (COMPLETE)", JobStatus{State: StateComplete}},
		{"does not panic on dropped connection (FAILED)", JobStatus{State: StateFailed, Error: "something broke"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kv := test.NewMockKV()
			kv.Seed("job-1", mustMarshalStatus(t, tc.status))
			h := newHandler(kv)

			req := httptest.NewRequest(http.MethodGet, "/jobs/job-1/status", nil)
			req.SetPathValue("id", "job-1")

			assert.NotPanics(t, func() {
				h.PollJobStatus(test.NewDroppedConnectionWriter(), req)
			})
		})
	}
}
