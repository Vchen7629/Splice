//go:build unit

package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"video-upload/internal/handler"
	"video-upload/internal/test"

	"github.com/stretchr/testify/assert"
)

func newVideoHandler(StorageURL string, js *test.MockJS) *handler.VideoHandler {
	return &handler.VideoHandler{
		Logger:         test.SilentLogger(),
		JS:             js,
		KV:             &test.MockKV{},
		StorageURL:     StorageURL,
		MaxUploadBytes: 0,
	}
}

func TestUploadVideo(t *testing.T) {
	t.Run("Returns 400 when body is not a multipart form", func(t *testing.T) {
		h := newVideoHandler("http://localhost:1", &test.MockJS{})
		req := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader("plain text body"))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

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

			h.UploadVideo(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantMsg)
		})
	}

	t.Run("Returns 500 when saving the video file fails", func(t *testing.T) {
		// Null byte in path causes os.MkdirAll to fail
		h := newVideoHandler("\x00", &test.MockJS{})
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to save uploaded video")
	})

	t.Run("Does not publish to NATS when saving fails", func(t *testing.T) {
		js := &test.MockJS{}
		h := newVideoHandler("\x00", js)
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

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

			h.DownloadVideo(rec, req)

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

		h.DownloadVideo(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to fetch video")
	})
}
