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

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	stest "splice.com/go_services/internal/shared/test"
)

// Upload / download routes.

// startTestServer calls StartHttpApi on a free port against a stub storage
// backend and registers cleanup. Returns the server and the port it listens on.
func startTestServer(t *testing.T, kv jetstream.KeyValue) (*http.Server, string) {
	t.Helper()

	fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(fakeSrv.Close)

	httpPort := stest.FreePort(t)
	server := StartHttpApi(stest.SilentLogger(), nil, &MockJS{}, kv, httpPort, fakeSrv.URL, ServiceURLs{})
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

func newCancelHandler(kv jetstream.KeyValue, nc *nats.Conn) *cancelHandler {
	return &cancelHandler{
		logger: stest.SilentLogger(),
		kv:     kv,
		nc:     nc,
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
					stest.SilentLogger(), nil, &MockJS{}, NewMockKV(),
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
			{"GET on health route returns 200", http.MethodGet, "/health", http.StatusOK},
			{"GET on cancel route returns 405", http.MethodGet, "/jobs/abc", http.StatusMethodNotAllowed},
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
			stest.SilentLogger(), nil, &MockJS{}, NewMockKV(),
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

func TestCancelProcessing(t *testing.T) {
	t.Run("Returns 400 when job_id path param is missing", func(t *testing.T) {
		c := newCancelHandler(&MockKV{}, nil)
		req := httptest.NewRequest(http.MethodDelete, "/jobs/", nil)
		req.SetPathValue("id", "")
		rec := httptest.NewRecorder()

		c.cancelProcessingRoute(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "missing job_id")
	})

	// get already returns the errKeyNotFound so no need to mock error
	t.Run("Returns 404 when KV Get returns ErrKeyNotFound", func(t *testing.T) {
		c := newCancelHandler(&MockKV{}, nil)
		req := httptest.NewRequest(http.MethodDelete, "/jobs/id-2", nil)
		req.SetPathValue("id", "id-2")
		rec := httptest.NewRecorder()

		c.cancelProcessingRoute(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "job not found")
	})

	t.Run("Returns 500 when KV Get returns generic error", func(t *testing.T) {
		c := newCancelHandler(&MockKV{GetErr: errors.New("kv unavailable")}, nil)
		req := httptest.NewRequest(http.MethodDelete, "/jobs/id-2", nil)
		req.SetPathValue("id", "id-2")
		rec := httptest.NewRecorder()

		c.cancelProcessingRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to get job status")
	})

	t.Run("Returns 500 when malformed JSON in stored entry", func(t *testing.T) {
		kv := NewMockKV()
		kv.Seed("job-1", []byte("Not valid json{{"))
		c := newCancelHandler(kv, nil)
		req := httptest.NewRequest(http.MethodDelete, "/jobs/job-1", nil)
		req.SetPathValue("id", "job-1")
		rec := httptest.NewRecorder()

		c.cancelProcessingRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to unmarshall kv value into JobStatus")
	})

	tests := []struct {
		name  string
		state JobState
	}{
		{"COMPLETE", StateComplete},
		{"FAILED", StateFailed},
		{"CANCELLED", StateCancelled},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("terminal state %s returns 200 and update never called", tc.name), func(t *testing.T) {
			kv := NewMockKV()
			status, err := json.Marshal(JobStatus{State: tc.state, Stage: "scene-detector"})
			require.NoError(t, err)
			kv.Seed("job-2", status)

			c := newCancelHandler(kv, nil)
			req := httptest.NewRequest(http.MethodDelete, "/jobs/job-2", nil)
			req.SetPathValue("id", "job-2")
			rec := httptest.NewRecorder()

			c.cancelProcessingRoute(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), string(tc.state))
			assert.False(t, kv.UpdateCalled.Load())
		})
	}

	t.Run("Returns 500 when KV update returns non ErrKeyExists error", func(t *testing.T) {
		kv := NewMockKV()
		status, err := json.Marshal(JobStatus{State: StateProcessing, Stage: "scene-detector"})
		require.NoError(t, err)
		kv.Seed("job-3", status)
		kv.UpdateErr = errors.New("update failed")

		c := newCancelHandler(kv, nil)
		req := httptest.NewRequest(http.MethodDelete, "/jobs/job-3", nil)
		req.SetPathValue("id", "job-3")
		rec := httptest.NewRecorder()

		c.cancelProcessingRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to update current stage for jobID as CANCELLED")
	})
}
