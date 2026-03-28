//go:build unit

package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"video-upload/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newJobStatusHandler(tracker *service.CompletedJobs) *JobStatusHandler {
	return &JobStatusHandler{
		Logger:  slog.Default(),
		Tracker: tracker,
	}
}

func TestInvalidPathValue(t *testing.T) {
	t.Run("Returns 400 when job ID path param is missing", func(t *testing.T) {
		h := newJobStatusHandler(service.NewCompletedJobs())
		req := httptest.NewRequest(http.MethodGet, "/jobs/", nil)
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("Response body contains missing job_id message", func(t *testing.T) {
		h := newJobStatusHandler(service.NewCompletedJobs())
		req := httptest.NewRequest(http.MethodGet, "/jobs/", nil)
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		assert.Contains(t, rec.Body.String(), "missing job_id")
	})

	t.Run("Does not write JSON body on missing ID", func(t *testing.T) {
		h := newJobStatusHandler(service.NewCompletedJobs())
		req := httptest.NewRequest(http.MethodGet, "/jobs/", nil)
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		var resp jobStatusResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.Error(t, err, "body should not be valid JSON on bad request")
	})
}

func TestProcessingState(t *testing.T) {
	t.Run("Returns PROCESSING when job is not in the completed tracker", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		h := newJobStatusHandler(tracker)

		req := httptest.NewRequest(http.MethodGet, "/jobs/job-123", nil)
		req.SetPathValue("id", "job-123")
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp jobStatusResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "PROCESSING", resp.State)
	})

	t.Run("Returns COMPLETE when job is marked done in the tracker", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		tracker.AddJob("job-456")
		h := newJobStatusHandler(tracker)

		req := httptest.NewRequest(http.MethodGet, "/jobs/job-456", nil)
		req.SetPathValue("id", "job-456")
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp jobStatusResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "COMPLETE", resp.State)
	})

	t.Run("State changes to COMPLETE after job is added to tracker", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		h := newJobStatusHandler(tracker)

		req := httptest.NewRequest(http.MethodGet, "/jobs/job-789", nil)
		req.SetPathValue("id", "job-789")
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		var first jobStatusResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&first))
		assert.Equal(t, "PROCESSING", first.State)

		tracker.AddJob("job-789")

		req2 := httptest.NewRequest(http.MethodGet, "/jobs/job-789", nil)
		req2.SetPathValue("id", "job-789")
		rec2 := httptest.NewRecorder()

		h.PollJobStatus(rec2, req2)

		var second jobStatusResponse
		require.NoError(t, json.NewDecoder(rec2.Body).Decode(&second))
		assert.Equal(t, "COMPLETE", second.State)
	})

	t.Run("Different jobs have independent states", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		tracker.AddJob("done-job")
		h := newJobStatusHandler(tracker)

		reqDone := httptest.NewRequest(http.MethodGet, "/jobs/done-job", nil)
		reqDone.SetPathValue("id", "done-job")
		recDone := httptest.NewRecorder()
		h.PollJobStatus(recDone, reqDone)

		reqPending := httptest.NewRequest(http.MethodGet, "/jobs/pending-job", nil)
		reqPending.SetPathValue("id", "pending-job")
		recPending := httptest.NewRecorder()
		h.PollJobStatus(recPending, reqPending)

		var done, pending jobStatusResponse
		require.NoError(t, json.NewDecoder(recDone.Body).Decode(&done))
		require.NoError(t, json.NewDecoder(recPending.Body).Decode(&pending))

		assert.Equal(t, "COMPLETE", done.State)
		assert.Equal(t, "PROCESSING", pending.State)
	})
}

func TestApiResponse(t *testing.T) {
	t.Run("Content-Type is application/json", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		h := newJobStatusHandler(tracker)

		req := httptest.NewRequest(http.MethodGet, "/jobs/job-1", nil)
		req.SetPathValue("id", "job-1")
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	})

	t.Run("Response body contains the requested job_id", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		h := newJobStatusHandler(tracker)

		req := httptest.NewRequest(http.MethodGet, "/jobs/my-specific-job", nil)
		req.SetPathValue("id", "my-specific-job")
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		var resp jobStatusResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, "my-specific-job", resp.JobID)
	})

	t.Run("Response body is valid JSON with job_id and state fields", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		h := newJobStatusHandler(tracker)

		req := httptest.NewRequest(http.MethodGet, "/jobs/job-1", nil)
		req.SetPathValue("id", "job-1")
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		var resp jobStatusResponse
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.JobID)
		assert.NotEmpty(t, resp.State)
	})

	t.Run("Returns 200 for a valid request", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		h := newJobStatusHandler(tracker)

		req := httptest.NewRequest(http.MethodGet, "/jobs/job-1", nil)
		req.SetPathValue("id", "job-1")
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
