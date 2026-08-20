//go:build unit

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	stest "splice.com/go_services/internal/shared/test"
)

func newHandler(kv *MockKV, urls ...ServiceURLs) *JobStatusHandler {
	var u ServiceURLs
	if len(urls) > 0 {
		u = urls[0]
	}
	return &JobStatusHandler{Logger: stest.SilentLogger(), KV: kv, URLs: u}
}

func mustMarshalStatus(t *testing.T, status JobStatus) []byte {
	t.Helper()
	b, err := json.Marshal(status)
	require.NoError(t, err)
	return b
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

func TestPollJobStatus_HealthCheck(t *testing.T) {
	healthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthySrv.Close()

	tests := []struct {
		name      string
		status    JobStatus
		urls      ServiceURLs
		wantState JobState
	}{
		{
			name:      "PROCESSING with service down becomes DEGRADED",
			status:    JobStatus{State: StateProcessing, Stage: "scene-detector"},
			urls:      ServiceURLs{Transcoder: "http://localhost:19999"},
			wantState: StateDegraded,
		},
		{
			name:      "PROCESSING with service up stays PROCESSING",
			status:    JobStatus{State: StateProcessing, Stage: "scene-detector"},
			urls:      ServiceURLs{Transcoder: healthySrv.URL},
			wantState: StateProcessing,
		},
		{
			name:      "DEGRADED with service recovered returns PROCESSING",
			status:    JobStatus{State: StateDegraded, Stage: "scene-detector", Error: "service unavailable at stage: transcoder"},
			urls:      ServiceURLs{Transcoder: healthySrv.URL},
			wantState: StateProcessing,
		},
		{
			name:      "COMPLETE skips health check",
			status:    JobStatus{State: StateComplete},
			urls:      ServiceURLs{},
			wantState: StateComplete,
		},
		{
			name:      "FAILED skips health check",
			status:    JobStatus{State: StateFailed, Error: "pipeline failed"},
			urls:      ServiceURLs{},
			wantState: StateFailed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kv := NewMockKV()
			kv.Seed("job-1", mustMarshalStatus(t, tc.status))
			h := &JobStatusHandler{Logger: stest.SilentLogger(), KV: kv, URLs: tc.urls}

			req := httptest.NewRequest(http.MethodGet, "/jobs/job-1/status", nil)
			req.SetPathValue("id", "job-1")
			rec := httptest.NewRecorder()

			h.PollJobStatus(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			var resp jobStatusResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, tc.wantState, resp.State)
		})
	}

	t.Run("updateJobStatusKV failure during health check serves", func(t *testing.T) {
		kv := NewMockKV()
		kv.Seed("job-1", mustMarshalStatus(t, JobStatus{State: StateProcessing, Stage: "scene-detector"}))
		kv.PutErr = errors.New("kv unavailable")
		h := &JobStatusHandler{Logger: stest.SilentLogger(), KV: kv, URLs: ServiceURLs{Transcoder: "http://localhost:19999"}}

		req := httptest.NewRequest(http.MethodGet, "/jobs/job-1/status", nil)
		req.SetPathValue("id", "job-1")
		rec := httptest.NewRecorder()

		h.PollJobStatus(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp jobStatusResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Equal(t, StateDegraded, resp.State) // service down -> degraded, served even though KV persist failed
	})
}

// Upload / download routes.

// startTestServer calls StartHttpApi on a free port against a stub storage
// backend and registers cleanup. Returns the server and the port it listens on.
func startTestServer(t *testing.T, kv jetstream.KeyValue) (*http.Server, string) {
	t.Helper()

	fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(fakeSrv.Close)

	httpPort := stest.FreePort(t)
	server := StartHttpApi(stest.SilentLogger(), &MockJS{}, kv, httpPort, fakeSrv.URL, ServiceURLs{})
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	return server, httpPort
}

func newVideoHandler(storageURL string, js *MockJS) *videoHandler {
	return &videoHandler{
		logger:         stest.SilentLogger(),
		js:             js,
		kv:             &MockKV{},
		storageURL:     storageURL,
		maxUploadBytes: 0,
	}
}

func TestStartHttp(t *testing.T) {
	t.Run("returns non-nil server with address derived from config", func(t *testing.T) {
		server, httpPort := startTestServer(t, &MockKV{})

		require.NotNil(t, server)
		assert.Equal(t, ":"+httpPort, server.Addr)
	})

	t.Run("server handler is non-nil", func(t *testing.T) {
		server, _ := startTestServer(t, &MockKV{})

		assert.NotNil(t, server.Handler)
	})

	t.Run("unregistered path returns 404", func(t *testing.T) {
		server, _ := startTestServer(t, &MockKV{})

		req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
		w := httptest.NewRecorder()
		server.Handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("server addr reflects configured port", func(t *testing.T) {
		tests := []struct {
			name     string
			httpPort string
			wantAddr string
		}{
			{"os-assigned port", "0", ":0"},
			{"explicit port", stest.FreePort(t), ""},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				wantAddr := tc.wantAddr
				if wantAddr == "" {
					wantAddr = ":" + tc.httpPort
				}

				server := StartHttpApi(
					stest.SilentLogger(), &MockJS{}, NewMockKV(),
					tc.httpPort, "http://localhost:1", ServiceURLs{},
				)
				t.Cleanup(func() { _ = server.Close() })

				assert.Equal(t, wantAddr, server.Addr)
			})
		}
	})
}

func TestStartHttpApiRouting(t *testing.T) {
	t.Run("route registration", func(t *testing.T) {
		tests := []struct {
			name       string
			method     string
			path       string
			wantStatus int
		}{
			{"POST on status route returns 405", http.MethodPost, "/jobs/abc/status", http.StatusMethodNotAllowed},
			{"PUT on status route returns 405", http.MethodPut, "/jobs/abc/status", http.StatusMethodNotAllowed},
			{"DELETE on status route returns 405", http.MethodDelete, "/jobs/abc/status", http.StatusMethodNotAllowed},
			{"GET on upload route returns 405", http.MethodGet, "/jobs/upload", http.StatusMethodNotAllowed},
			{"GET on download route returns 405", http.MethodGet, "/jobs/download", http.StatusMethodNotAllowed},
			{"path missing status segment returns 404", http.MethodGet, "/jobs/abc", http.StatusNotFound},
			{"completely unknown path returns 404", http.MethodGet, "/healthz", http.StatusNotFound},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				server, _ := startTestServer(t, NewMockKV())

				req := httptest.NewRequest(tc.method, tc.path, nil)
				rec := httptest.NewRecorder()
				server.Handler.ServeHTTP(rec, req)

				assert.Equal(t, tc.wantStatus, rec.Code)
			})
		}
	})

	t.Run("CORS middleware is wired", func(t *testing.T) {
		tests := []struct {
			name            string
			origin          string
			wantStatus      int
			wantAllowOrigin string
		}{
			{
				name:            "OPTIONS from allowed origin returns 204 with CORS header",
				origin:          "http://localhost:5173",
				wantStatus:      http.StatusNoContent,
				wantAllowOrigin: "http://localhost:5173",
			},
			{
				name:       "OPTIONS from disallowed origin returns 403 with no CORS header",
				origin:     "http://evil.com",
				wantStatus: http.StatusForbidden,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				server, _ := startTestServer(t, NewMockKV())

				req := httptest.NewRequest(http.MethodOptions, "/jobs/abc/status", nil)
				req.Header.Set("Origin", tc.origin)
				rec := httptest.NewRecorder()
				server.Handler.ServeHTTP(rec, req)

				assert.Equal(t, tc.wantStatus, rec.Code)
				assert.Equal(t, tc.wantAllowOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
			})
		}
	})

	t.Run("calls osExit(1) when port is already in use", func(t *testing.T) {
		ln, err := net.Listen("tcp", ":0")
		require.NoError(t, err)
		defer ln.Close() //nolint:errcheck
		port := ln.Addr().(*net.TCPAddr).Port

		exitCode := patchOsExit(t)
		server := StartHttpApi(
			stest.SilentLogger(), &MockJS{}, NewMockKV(),
			fmt.Sprintf("%d", port), "http://localhost:1", ServiceURLs{},
		)
		t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

		require.Eventually(t, func() bool {
			return *exitCode == 1
		}, 2*time.Second, 10*time.Millisecond, "expected osExit(1) to be called")
	})
}

func TestUploadVideo(t *testing.T) {
	t.Run("Returns 400 when body is not a multipart form", func(t *testing.T) {
		h := newVideoHandler("http://localhost:1", &MockJS{})
		req := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader("plain text body"))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid multipart form")
	})

	cases := []struct {
		name      string
		fileName  string
		content   []byte
		targetRes string
		wantMsg   string
	}{
		{"video field is missing", "", nil, "1080p", "missing video field"},
		{"target_resolution is missing", "video.mp4", []byte("data"), "", "missing target_resolution field"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newVideoHandler("http://localhost:1", &MockJS{})
			req := NewUploadRequest(t, "/jobs", tc.fileName, tc.content, tc.targetRes, "1080p", "Transcode")
			rec := httptest.NewRecorder()

			h.uploadVideoRoute(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantMsg)
		})
	}

	t.Run("Returns 500 when saving the video file fails", func(t *testing.T) {
		// Null byte in the storage URL makes the upload request fail to build.
		h := newVideoHandler("\x00", &MockJS{})
		req := NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p", "1080p", "Transcode")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to save uploaded video")
	})

	t.Run("returns 500 when KV.Put fails during upload", func(t *testing.T) {
		kv := &MockKV{PutErr: errors.New("kv unavailable")}
		server, _ := startTestServer(t, kv)

		req := NewUploadRequest(t, "/jobs/upload", "video.mp4", []byte("data"), "1080p", "1080p", "Transcode")
		w := httptest.NewRecorder()
		server.Handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to record job status")
	})

	t.Run("Does not publish to NATS when saving fails", func(t *testing.T) {
		js := &MockJS{}
		h := newVideoHandler("\x00", js)
		req := NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p", "1080p", "Transcode")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.False(t, js.PublishCalled, "publish should not be called when save fails")
	})
}

func TestDownloadVideo(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"invalid json", `not json`, "invalid json payload"},
		{"missing job_id", `{"file_name":"video.mp4"}`, ""},
		{"missing file_name", `{"job_id":"abc-123"}`, ""},
		{"job_id too short", `{"job_id":"a","file_name":"video.mp4"}`, ""},
		{"file_name too short", `{"job_id":"abc-123","file_name":"v"}`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newVideoHandler("http://localhost:1", &MockJS{})
			req := httptest.NewRequest(http.MethodGet, "/jobs", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			h.downloadVideoRoute(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			if tc.wantMsg != "" {
				assert.Contains(t, rec.Body.String(), tc.wantMsg)
			}
		})
	}

	t.Run("Returns 500 when storage is unreachable", func(t *testing.T) {
		h := newVideoHandler("http://localhost:1", &MockJS{})
		req := NewDownloadRequest(t, "/jobs/download", "abc-123", "video.mp4")
		rec := httptest.NewRecorder()

		h.downloadVideoRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to fetch video")
	})
}
