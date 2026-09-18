//go:build unit

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/test"

	"github.com/nats-io/nats.go/jetstream"
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

func mustMarshalStatus(t *testing.T, status sJetstream.JobStatus) []byte {
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
			err := h.updateJobStatusKV(context.Background(), "job-1", sJetstream.JobStatus{State: sJetstream.StateProcessing, Stage: "scene-detector"})

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTryUpdateMilestone(t *testing.T) {
	writePolicyTests := []struct {
		name        string
		current     *test.MockKV
		newStatus   sJetstream.JobStatus
		wantOutcome milestoneWriteOutcome
	}{
		{
			name:        "not found when job has no milestone yet",
			current:     &test.MockKV{},
			newStatus:   sJetstream.JobStatus{State: "PROCESSING", Stage: "transcoder"},
			wantOutcome: milestoneNotFound,
		},
		{
			name:        "writes when new stage is ahead of current",
			current:     &test.MockKV{GetFound: true, GetValue: []byte(`{"state":"PROCESSING","stage":"transcoder"}`)},
			newStatus:   sJetstream.JobStatus{State: "PROCESSING", Stage: "video-recombiner"},
			wantOutcome: milestoneWritten,
		},
		{
			name:        "skips when new stage is behind current",
			current:     &test.MockKV{GetFound: true, GetValue: []byte(`{"state":"PROCESSING","stage":"video-recombiner"}`)},
			newStatus:   sJetstream.JobStatus{State: "PROCESSING", Stage: "transcoder"},
			wantOutcome: milestoneSkipped,
		},
		{
			name:        "skips on existing terminal COMPLETE",
			current:     &test.MockKV{GetFound: true, GetValue: []byte(`{"state":"COMPLETE","stage":""}`)},
			newStatus:   sJetstream.JobStatus{State: "PROCESSING", Stage: "transcoder"},
			wantOutcome: milestoneSkipped,
		},
		{
			name:        "skips on existing terminal FAILED",
			current:     &test.MockKV{GetFound: true, GetValue: []byte(`{"state":"FAILED","stage":"upload"}`)},
			newStatus:   sJetstream.JobStatus{State: "PROCESSING", Stage: "transcoder"},
			wantOutcome: milestoneSkipped,
		},
		{
			name:        "skips on existing terminal CANCELLED",
			current:     &test.MockKV{GetFound: true, GetValue: []byte(`{"state":"CANCELLED","stage":"upload"}`)},
			newStatus:   sJetstream.JobStatus{State: "PROCESSING", Stage: "transcoder"},
			wantOutcome: milestoneSkipped,
		},
		{
			// root-cause case: a terminal write must go through even though the stage-order
			// guard would otherwise reject it (a terminal newStatus has no ranked stage of its
			// own unless inherited, so it must bypass the ordinal comparison entirely).
			name:        "writes terminal status even when current stage is furthest along",
			current:     &test.MockKV{GetFound: true, GetValue: []byte(`{"state":"PROCESSING","stage":"video-recombiner"}`)},
			newStatus:   sJetstream.JobStatus{State: "CANCELLED"},
			wantOutcome: milestoneWritten,
		},
	}

	for _, tc := range writePolicyTests {
		t.Run(tc.name, func(t *testing.T) {
			_, outcome, err := tryUpdateMilestone(context.Background(), tc.current, "job-1", tc.newStatus)

			require.NoError(t, err)
			assert.Equal(t, tc.wantOutcome, outcome)
			assert.Empty(t, tc.current.CreateKey, "tryUpdateMilestone must never create a missing entry")
		})
	}

	t.Run("inherits current stage when newStatus.Stage is empty", func(t *testing.T) {
		mockKV := &test.MockKV{GetFound: true, GetValue: []byte(`{"state":"PROCESSING","stage":"transcoder"}`)}

		result, outcome, err := tryUpdateMilestone(context.Background(), mockKV, "job-1", sJetstream.JobStatus{State: "COMPLETE"})

		require.NoError(t, err)
		assert.Equal(t, milestoneWritten, outcome)
		assert.Equal(t, "transcoder", result.Stage)
	})

	t.Run("keeps explicit stage over the inherited current stage", func(t *testing.T) {
		mockKV := &test.MockKV{GetFound: true, GetValue: []byte(`{"state":"PROCESSING","stage":"transcoder"}`)}

		result, outcome, err := tryUpdateMilestone(context.Background(), mockKV, "job-1", sJetstream.JobStatus{State: "COMPLETE", Stage: "video-recombiner"})

		require.NoError(t, err)
		assert.Equal(t, milestoneWritten, outcome)
		assert.Equal(t, "video-recombiner", result.Stage)
	})

	// error tests

	transcoderValue := []byte(`{"state":"PROCESSING","stage":"transcoder"}`)

	errorTests := []struct {
		name   string
		mockKV *test.MockKV
	}{
		{"Get fails", &test.MockKV{GetErr: errors.New("kv unavailable")}},
		{"malformed JSON in stored entry", &test.MockKV{GetFound: true, GetValue: []byte("not valid json{{")}},
		{"Update fails", &test.MockKV{GetFound: true, GetValue: transcoderValue, UpdateErr: errors.New("update failed")}},
	}

	for _, tc := range errorTests {
		t.Run(tc.name, func(t *testing.T) {
			newStatus := sJetstream.JobStatus{State: "PROCESSING", Stage: "video-recombiner"}

			_, outcome, err := tryUpdateMilestone(context.Background(), tc.mockKV, "job-1", newStatus)

			require.Error(t, err)
			assert.Equal(t, milestoneError, outcome)
		})
	}

	t.Run("Update conflict surfaces ErrKeyExists unchanged (no internal retry)", func(t *testing.T) {
		mockKV := &test.MockKV{GetFound: true, GetValue: transcoderValue, UpdateErr: jetstream.ErrKeyExists}
		newStatus := sJetstream.JobStatus{State: "PROCESSING", Stage: "video-recombiner"}

		_, outcome, err := tryUpdateMilestone(context.Background(), mockKV, "job-1", newStatus)

		require.Error(t, err)
		assert.ErrorIs(t, err, jetstream.ErrKeyExists)
		assert.Equal(t, milestoneError, outcome)
	})
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
		status     sJetstream.JobStatus
		wantState  sJetstream.JobState
		wantErrMsg string
	}{
		{
			name:      "PROCESSING state",
			status:    sJetstream.JobStatus{State: sJetstream.StateProcessing, Stage: "scene-detector"},
			wantState: sJetstream.StateProcessing,
		},
		{
			name:      "COMPLETE state",
			status:    sJetstream.JobStatus{State: sJetstream.StateComplete, Stage: "scene-detector"},
			wantState: sJetstream.StateComplete,
		},
		{
			name:      "CANCELLED state",
			status:    sJetstream.JobStatus{State: sJetstream.StateCancelled, Stage: "scene-detector"},
			wantState: sJetstream.StateCancelled,
		},
		{
			name:       "FAILED state includes error message",
			status:     sJetstream.JobStatus{State: sJetstream.StateFailed, Stage: "scene-detector", Error: "pipeline failed at stage: transcoder-worker"},
			wantState:  sJetstream.StateFailed,
			wantErrMsg: "pipeline failed at stage: transcoder-worker",
		},
		{
			name:      "FAILED with empty error field",
			status:    sJetstream.JobStatus{State: sJetstream.StateFailed, Stage: "transcoder"},
			wantState: sJetstream.StateFailed,
		},
		{
			name:       "DEGRADED state includes error message",
			status:     sJetstream.JobStatus{State: sJetstream.StateDegraded, Stage: "scene-detector", Error: "service unavailable at stage: transcoder"},
			wantState:  sJetstream.StateDegraded,
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
			kv.Seed(tc.jobID, mustMarshalStatus(t, sJetstream.JobStatus{State: sJetstream.StateProcessing}))
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
		status sJetstream.JobStatus
	}{
		{"does not panic on dropped connection (PROCESSING)", sJetstream.JobStatus{State: sJetstream.StateProcessing, Stage: "scene-detector"}},
		{"does not panic on dropped connection (COMPLETE)", sJetstream.JobStatus{State: sJetstream.StateComplete, Stage: "scene-detector"}},
		{"does not panic on dropped connection (FAILED)", sJetstream.JobStatus{State: sJetstream.StateFailed, Stage: "transcoder", Error: "something broke"}},
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
