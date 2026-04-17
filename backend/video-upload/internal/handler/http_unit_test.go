//go:build unit

package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"video-upload/internal/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freePort returns a port number that is not currently in use.
func freePort(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)

	err = l.Close()
	require.NoError(t, err)

	return port
}

// startTestServer calls startHttpApi with a free port and a temp output dir,
// registers a Cleanup to shut the server down, and returns the server and cfg.
func startTestServer(t *testing.T, kv jetstream.KeyValue) (*http.Server, string) {
	t.Helper()

	fakeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	t.Cleanup(fakeSrv.Close)

	HTTPPort := freePort(t)
	server := StartHttpApi(test.SilentLogger(), &test.MockJS{}, kv, HTTPPort, fakeSrv.URL)

	t.Cleanup(func() { server.Shutdown(context.Background()) }) //nolint:errcheck

	return server, HTTPPort
}

func TestStartHttp(t *testing.T) {
	t.Run("returns non-nil server with address derived from config", func(t *testing.T) {
		server, HTTPPort := startTestServer(t, &test.MockKV{})

		require.NotNil(t, server)
		assert.Equal(t, ":"+HTTPPort, server.Addr)
	})

	t.Run("server handler is non-nil", func(t *testing.T) {
		server, _ := startTestServer(t, &test.MockKV{})

		assert.NotNil(t, server.Handler)
	})

	t.Run("unregistered path returns 404", func(t *testing.T) {
		server, _ := startTestServer(t, &test.MockKV{})

		req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
		w := httptest.NewRecorder()
		server.Handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func newVideoHandler(StorageURL string, js *test.MockJS) *videoHandler {
	return &videoHandler{
		logger:         test.SilentLogger(),
		js:             js,
		kv:             &test.MockKV{},
		storageURL:     StorageURL,
		maxUploadBytes: 0,
	}
}

func TestUploadVideo(t *testing.T) {
	t.Run("Returns 400 when body is not a multipart form", func(t *testing.T) {
		h := newVideoHandler("http://localhost:1", &test.MockJS{})
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
			h := newVideoHandler("http://localhost:1", &test.MockJS{})
			req := test.NewUploadRequest(t, "/jobs", tc.fileName, tc.content, tc.targetRes)
			rec := httptest.NewRecorder()

			h.uploadVideoRoute(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantMsg)
		})
	}

	t.Run("Returns 500 when saving the video file fails", func(t *testing.T) {
		// Null byte in path causes os.MkdirAll to fail
		h := newVideoHandler("\x00", &test.MockJS{})
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to save uploaded video")
	})

	t.Run("returns 500 when KV.Put fails during upload", func(t *testing.T) {
		kv := &test.MockKV{PutErr: errors.New("kv unavailable")}
		server, _ := startTestServer(t, kv)

		req := test.NewUploadRequest(t, "/jobs/upload", "video.mp4", []byte("data"), "1080p")
		w := httptest.NewRecorder()
		server.Handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to record job status")
	})

	t.Run("Does not publish to NATS when saving fails", func(t *testing.T) {
		js := &test.MockJS{}
		h := newVideoHandler("\x00", js)
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
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
			h := newVideoHandler("http://localhost:1", &test.MockJS{})
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
		h := newVideoHandler("http://localhost:1", &test.MockJS{})
		req := test.NewDownloadRequest(t, "abc-123", "video.mp4")
		rec := httptest.NewRecorder()

		h.downloadVideoRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to fetch video")
	})
}
