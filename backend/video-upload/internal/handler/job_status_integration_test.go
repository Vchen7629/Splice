//go:build integration

package handler_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"video-upload/internal/handler"
	"video-upload/internal/service"
	"video-upload/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Smoke test: confirms routing + handler + tracker work correctly end-to-end over real HTTP.
func TestResponse(t *testing.T) {
	t.Run("Full stack returns correct response for a known job", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		tracker.AddJob("job-1")
		ts := test.NewTestServer(tracker)
		defer ts.Close()

		resp, err := http.Get(fmt.Sprintf("%s/jobs/job-1", ts.URL))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var body struct {
			JobID string `json:"job_id"`
			State string `json:"state"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "job-1", body.JobID)
		assert.Equal(t, "COMPLETE", body.State)
	})

	t.Run("Mux returns 404 when no ID segment is in the path", func(t *testing.T) {
		ts := test.NewTestServer(service.NewCompletedJobs())
		defer ts.Close()

		// "GET /jobs/{id}" requires a non-empty segment — the mux won't route /jobs/
		resp, err := http.Get(fmt.Sprintf("%s/jobs/", ts.URL))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestConnectionDrop(t *testing.T) {
	t.Run("Handler does not panic when the connection drops before write (PROCESSING)", func(t *testing.T) {
		h := &handler.JobStatusHandler{
			Logger:  slog.Default(),
			Tracker: service.NewCompletedJobs(),
		}
		req := httptest.NewRequest(http.MethodGet, "/jobs/job-1", nil)
		req.SetPathValue("id", "job-1")

		assert.NotPanics(t, func() {
			h.PollJobStatus(test.NewDroppedConnectionWriter(), req)
		})
	})

	t.Run("Handler does not panic when the connection drops before write (COMPLETE)", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		tracker.AddJob("done-job")
		h := &handler.JobStatusHandler{
			Logger:  slog.Default(),
			Tracker: tracker,
		}
		req := httptest.NewRequest(http.MethodGet, "/jobs/done-job", nil)
		req.SetPathValue("id", "done-job")

		assert.NotPanics(t, func() {
			h.PollJobStatus(test.NewDroppedConnectionWriter(), req)
		})
	})

	t.Run("Server continues serving requests after a client disconnects", func(t *testing.T) {
		ts := test.NewTestServer(service.NewCompletedJobs())
		defer ts.Close()

		firstResp, err := http.Get(fmt.Sprintf("%s/jobs/job-1", ts.URL))
		require.NoError(t, err)
		firstResp.Body.Close()

		secondResp, err := http.Get(fmt.Sprintf("%s/jobs/job-1", ts.URL))
		require.NoError(t, err)
		defer secondResp.Body.Close()

		assert.Equal(t, http.StatusOK, secondResp.StatusCode)
	})
}

func TestPollJobStatus_ConcurrentRequests(t *testing.T) {
	t.Run("Concurrent real HTTP requests for a completed job return consistent state", func(t *testing.T) {
		tracker := service.NewCompletedJobs()
		tracker.AddJob("concurrent-job")
		ts := test.NewTestServer(tracker)
		defer ts.Close()

		const goroutines = 20
		results := make([]string, goroutines)
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := range goroutines {
			go func(idx int) {
				defer wg.Done()
				resp, err := http.Get(fmt.Sprintf("%s/jobs/concurrent-job", ts.URL))
				if err != nil {
					return
				}
				defer resp.Body.Close()
				var body struct{ State string `json:"state"` }
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
}
