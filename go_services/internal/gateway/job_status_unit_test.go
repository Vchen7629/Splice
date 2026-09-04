//go:build unit

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHandler(kv *MockKV, urls ...ServiceURLs) *JobStatusHandler {
	var u ServiceURLs
	if len(urls) > 0 {
		u = urls[0]
	}
	return &JobStatusHandler{Logger: test.SilentLogger(), KV: kv, URLs: u}
}

func mustMarshalStatus(t *testing.T, status JobStatus) []byte {
	t.Helper()
	b, err := json.Marshal(status)
	require.NoError(t, err)
	return b
}

func TestGetJobStatusKV(t *testing.T) {
	tests := []struct {
		name       string
		kv         *MockKV
		wantStatus int
		wantErr    string
	}{
		{
			name:       "key not found returns 404",
			kv:         NewMockKV(),
			wantStatus: http.StatusNotFound,
			wantErr:    "job not found",
		},
		{
			name: "generic KV error returns 500",
			kv: func() *MockKV {
				m := NewMockKV()
				m.GetErr = errors.New("kv unavailable")
				return m
			}(),
			wantStatus: http.StatusInternalServerError,
			wantErr:    "failed to get job status",
		},
		{
			name: "success returns entry and 200",
			kv: func() *MockKV {
				m := NewMockKV()
				m.Seed("job-1", []byte(`{"state":"PROCESSING"}`))
				return m
			}(),
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &KVHandler{logger: test.SilentLogger(), kv: tc.kv}
			entry, code, err := h.getJobStatusKV(context.Background(), "job-1")

			assert.Equal(t, tc.wantStatus, code)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Nil(t, entry)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, entry)
			}
		})
	}
}

func TestUpdateJobStatusKV(t *testing.T) {
	tests := []struct {
		name    string
		kv      *MockKV
		wantErr bool
	}{
		{name: "success returns nil", kv: NewMockKV(), wantErr: false},
		{
			name: "KV Put error returns error",
			kv: func() *MockKV {
				m := NewMockKV()
				m.PutErr = errors.New("kv unavailable")
				return m
			}(),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &KVHandler{logger: test.SilentLogger(), kv: tc.kv}
			err := h.updateJobStatusKV(context.Background(), "job-1", JobStatus{State: StateProcessing, Stage: "scene-detector"})

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPollJobStatus_BadRequest(t *testing.T) {
	h := newHandler(NewMockKV())
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
		kv         *MockKV
		wantStatus int
		wantBody   string
	}{
		{
			name:       "key not found returns 404",
			kv:         NewMockKV(),
			wantStatus: http.StatusNotFound,
			wantBody:   "job not found",
		},
		{
			name: "generic KV error returns 500",
			kv: func() *MockKV {
				m := NewMockKV()
				m.GetErr = kvErr
				return m
			}(),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "failed to get job status",
		},
		{
			name: "malformed KV value returns 500",
			kv: func() *MockKV {
				m := NewMockKV()
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
			status:    JobStatus{State: StateProcessing, Stage: "scene-detector"},
			wantState: StateProcessing,
		},
		{
			name:      "COMPLETE state",
			status:    JobStatus{State: StateComplete, Stage: "scene-detector"},
			wantState: StateComplete,
		},
		{
			name:      "CANCELLED state",
			status:    JobStatus{State: StateCancelled, Stage: "scene-detector"},
			wantState: StateCancelled,
		},
		{
			name:       "FAILED state includes error message",
			status:     JobStatus{State: StateFailed, Stage: "scene-detector", Error: "pipeline failed at stage: transcoder-worker"},
			wantState:  StateFailed,
			wantErrMsg: "pipeline failed at stage: transcoder-worker",
		},
		{
			name:      "FAILED with empty error field",
			status:    JobStatus{State: StateFailed, Stage: "transcoder"},
			wantState: StateFailed,
		},
		{
			name:       "DEGRADED state includes error message",
			status:     JobStatus{State: StateDegraded, Stage: "scene-detector", Error: "service unavailable at stage: transcoder"},
			wantState:  StateDegraded,
			wantErrMsg: "service unavailable at stage: transcoder",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kv := NewMockKV()
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
		name      string
		jobID     string
		wantStage string
	}{
		{"echoes job_id in response", "my-specific-job", ""},
		{"echoes different job_id", "another-job-456", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kv := NewMockKV()
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
			assert.Equal(t, tc.wantStage, resp.Stage)
		})
	}
}

func TestPollJobStatus_DroppedConnection(t *testing.T) {
	tests := []struct {
		name   string
		status JobStatus
	}{
		{"does not panic on dropped connection (PROCESSING)", JobStatus{State: StateProcessing, Stage: "scene-detector"}},
		{"does not panic on dropped connection (COMPLETE)", JobStatus{State: StateComplete, Stage: "scene-detector"}},
		{"does not panic on dropped connection (FAILED)", JobStatus{State: StateFailed, Stage: "transcoder", Error: "something broke"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kv := NewMockKV()
			kv.Seed("job-1", mustMarshalStatus(t, tc.status))
			h := newHandler(kv)

			req := httptest.NewRequest(http.MethodGet, "/jobs/job-1/status", nil)
			req.SetPathValue("id", "job-1")

			assert.NotPanics(t, func() {
				h.PollJobStatus(newDroppedConnectionWriter(), req)
			})
		})
	}
}

func TestJobEvents_MissingJobID(t *testing.T) {
	h := newHandler(NewMockKV())

	req := httptest.NewRequest(http.MethodGet, "/jobs//events", nil)
	req.SetPathValue("id", "")
	rec := httptest.NewRecorder()

	h.JobEvents(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing job_id")
}

func TestJobEvents_WatchError(t *testing.T) {
	kv := &MockKV{WatchErr: errors.New("kv unavailable")}
	h := newHandler(kv)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-1/events", nil)
	req.SetPathValue("id", "job-1")
	rec := httptest.NewRecorder()

	h.JobEvents(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "failed to watch job status")
}
