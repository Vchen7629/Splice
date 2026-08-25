//go:build integration

package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/handler"
)

type sseEvent struct {
	Event string
	Data  string
}

// reads the next SSE frame from r
func waitForSSEEvent(t *testing.T, r *bufio.Reader, timeout time.Duration) sseEvent {
	t.Helper()

	type result struct {
		ev  sseEvent
		err error
	}
	ch := make(chan result, 1)
	go func() {
		var ev sseEvent
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				ch <- result{ev, err}
				return
			}
			line = strings.TrimRight(line, "\n")
			if line == "" {
				ch <- result{ev, nil}
				return
			}
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.Data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	select {
	case res := <-ch:
		require.NoError(t, res.err)
		return res.ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for SSE event")
		return sseEvent{}
	}
}

// opens a GET to /jobs/{jobID}/events, assert the response is a live SSE stream
// and returns it ready to read frames from
func connectSSE(t *testing.T, ctx context.Context, ts *httptest.Server, jobID string) (*http.Response, *bufio.Reader) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/jobs/"+jobID+"/events", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	return resp, bufio.NewReader(resp.Body)
}

// assert ev's event name and unmarshals its data into T
func decodeSSEEvent[T any](t *testing.T, ev sseEvent, wantEvent string) T {
	t.Helper()
	require.Equal(t, wantEvent, ev.Event)
	var v T
	require.NoError(t, json.Unmarshal([]byte(ev.Data), &v))

	return v
}

func TestJobEvents_FullLifecycle(t *testing.T) {
	jobID := "job-events-lifecycle"
	seedStatus(t, jobID, JobStatus{State: StateProcessing, Stage: "upload"})
	ts := newTestServer(t, ServiceURLs{})
	_, r := connectSSE(t, context.Background(), ts, jobID)

	// initial snapshot
	status := decodeSSEEvent[jobStatusResponse](t, waitForSSEEvent(t, r, 5*time.Second), "status")
	assert.Equal(t, jobID, status.JobID)
	assert.Equal(t, "PROCESSING", string(status.State))
	assert.Equal(t, "upload", status.Stage)

	// progress tick over NATS
	progressMsg := handler.ProgressMessage{JobID: jobID, Stage: "upload", Progress: 42}
	data, err := json.Marshal(progressMsg)
	require.NoError(t, err)
	require.NoError(t, sharedNC.Publish("progress."+jobID, data))

	got := decodeSSEEvent[handler.ProgressMessage](t, waitForSSEEvent(t, r, 5*time.Second), "progress")
	assert.Equal(t, 42, got.Progress)

	// stage transition
	seedStatus(t, jobID, JobStatus{State: StateProcessing, Stage: "scene-detector"})
	status = decodeSSEEvent[jobStatusResponse](t, waitForSSEEvent(t, r, 5*time.Second), "status")
	assert.Equal(t, "scene-detector", status.Stage)

	// terminal state closes the conn
	seedStatus(t, jobID, JobStatus{State: StateComplete})
	status = decodeSSEEvent[jobStatusResponse](t, waitForSSEEvent(t, r, 5*time.Second), "status")
	assert.Equal(t, "COMPLETE", string(status.State))

	_, err = r.ReadString('\n')
	assert.ErrorIs(t, err, io.EOF)
}

// covers both DEGRADED/PROCESSING flip and that unchanged health reading doesnt emit duplicate event
func TestJobEvents_HealthFlip(t *testing.T) {
	var healthy atomic.Bool
	stageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer stageSrv.Close()

	jobID := "job-events-health"
	seedStatus(t, jobID, JobStatus{State: StateProcessing, Stage: "scene-detector"})
	ts := newTestServer(t, ServiceURLs{SceneDetector: stageSrv.URL})
	_, r := connectSSE(t, context.Background(), ts, jobID)

	waitForSSEEvent(t, r, 5*time.Second)

	h := decodeSSEEvent[healthEvent](t, waitForSSEEvent(t, r, 5*time.Second), "health")
	assert.Equal(t, "DEGRADED", string(h.State))

	healthy.Store(true)
	seedStatus(t, jobID, JobStatus{State: StateProcessing, Stage: "scene-detector"})

	waitForSSEEvent(t, r, 5*time.Second) // status event for the re-Put
	h = decodeSSEEvent[healthEvent](t, waitForSSEEvent(t, r, 5*time.Second), "health")
	assert.Equal(t, "PROCESSING", string(h.State))

	seedStatus(t, jobID, JobStatus{State: StateProcessing, Stage: "scene-detector"})
	ev := waitForSSEEvent(t, r, 5*time.Second)
	assert.Equal(t, "status", ev.Event, "no duplicate health event should follow an unchanged reading")
}

func TestJobEvents_ClientDisconnect(t *testing.T) {
	jobID := "job-events-disconnect"
	seedStatus(t, jobID, JobStatus{State: StateProcessing, Stage: "upload"})
	ts := newTestServer(t, ServiceURLs{})

	ctx, cancel := context.WithCancel(context.Background())
	resp, r := connectSSE(t, ctx, ts, jobID)

	waitForSSEEvent(t, r, 5*time.Second)

	cancel()
	resp.Body.Close()

	// give the handler goroutine a moment to unwind, then confirm the server is still healthy
	seedStatus(t, jobID, JobStatus{State: StateComplete})
	time.Sleep(200 * time.Millisecond)

	statusResp, err := http.Get(ts.URL + "/jobs/" + jobID + "/status")
	require.NoError(t, err)
	defer statusResp.Body.Close()
	assert.Equal(t, http.StatusOK, statusResp.StatusCode)
}
