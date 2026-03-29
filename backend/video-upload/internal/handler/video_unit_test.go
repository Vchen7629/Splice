//go:build unit

package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"video-upload/internal/handler"
	"video-upload/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newVideoHandler(outputDir string, js *test.MockJS) *handler.VideoHandler {
	return &handler.VideoHandler{
		Logger:         test.SilentLogger(),
		JS:             js,
		OutputDir:      outputDir,
		MaxUploadBytes: 0,
	}
}

func TestDownloadVideo(t *testing.T) {
	t.Run("Returns 400 and missing job_id message when path param is empty", func(t *testing.T) {
		h := newVideoHandler(t.TempDir(), &test.MockJS{})
		req := httptest.NewRequest(http.MethodGet, "/jobs/", nil)
		rec := httptest.NewRecorder()

		h.DownloadVideo(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "missing job_id")
	})

	t.Run("Returns 404 when the output file does not exist", func(t *testing.T) {
		h := newVideoHandler(t.TempDir(), &test.MockJS{})
		req := httptest.NewRequest(http.MethodGet, "/jobs/nonexistent-job", nil)
		req.SetPathValue("id", "nonexistent-job")
		rec := httptest.NewRecorder()

		h.DownloadVideo(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("Returns 200 and serves the file when output.mp4 exists", func(t *testing.T) {
		dir := t.TempDir()
		jobID := "job-123"
		filePath := filepath.Join(dir, "jobs", jobID, "output.mp4")
		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755))
		require.NoError(t, os.WriteFile(filePath, []byte("fake video bytes"), 0644))

		h := newVideoHandler(dir, &test.MockJS{})
		req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID, nil)
		req.SetPathValue("id", jobID)
		rec := httptest.NewRecorder()

		h.DownloadVideo(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "fake video bytes", rec.Body.String())
	})

	t.Run("Constructs the path from outputDir/jobs/jobID/output.mp4", func(t *testing.T) {
		dir := t.TempDir()
		jobID := "abc-456"
		expectedPath := filepath.Join(dir, "jobs", jobID, "output.mp4")
		require.NoError(t, os.MkdirAll(filepath.Dir(expectedPath), 0755))
		require.NoError(t, os.WriteFile(expectedPath, []byte("data"), 0644))

		h := newVideoHandler(dir, &test.MockJS{})
		req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID, nil)
		req.SetPathValue("id", jobID)
		rec := httptest.NewRecorder()

		h.DownloadVideo(rec, req)

		// Correct path resolves to the file, wrong path would 404
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestUploadVideoMissingFields(t *testing.T) {
	t.Run("Returns 400 when body is not a multipart form", func(t *testing.T) {
		h := newVideoHandler(t.TempDir(), &test.MockJS{})
		req := httptest.NewRequest(http.MethodPost, "/videos", strings.NewReader("plain text body"))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid multipart form")
	})

	t.Run("Returns 400 when video field is missing from multipart form", func(t *testing.T) {
		h := newVideoHandler(t.TempDir(), &test.MockJS{})
		req := test.NewUploadRequest(t, "/jobs", "", nil, "1080p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "missing video field")
	})

	t.Run("Returns 400 when target_resolution is missing from multipart form", func(t *testing.T) {
		h := newVideoHandler(t.TempDir(), &test.MockJS{})
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "missing target_resolution field")
	})
}

func TestUploadVideoErrors(t *testing.T) {
	t.Run("Returns 500 when saving the video file fails", func(t *testing.T) {
		// Null byte in path causes os.MkdirAll to fail
		h := newVideoHandler("\x00", &test.MockJS{})
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to save uploaded video")
	})

	t.Run("Returns 500 when NATS publish fails", func(t *testing.T) {
		js := &test.MockJS{PublishErr: errors.New("nats unavailable")}
		h := newVideoHandler(t.TempDir(), js)
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "unable to send process request")
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

func TestUploadVideoSuccess(t *testing.T) {
	t.Run("Returns 201 on a valid upload", func(t *testing.T) {
		h := newVideoHandler(t.TempDir(), &test.MockJS{})
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("Response Content-Type is application/json", func(t *testing.T) {
		h := newVideoHandler(t.TempDir(), &test.MockJS{})
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	})

	t.Run("Response body contains a non-empty job_id", func(t *testing.T) {
		h := newVideoHandler(t.TempDir(), &test.MockJS{})
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		var resp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.NotEmpty(t, resp.JobID)
	})

	t.Run("Publishes to NATS on a successful upload", func(t *testing.T) {
		js := &test.MockJS{}
		h := newVideoHandler(t.TempDir(), js)
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")

		h.UploadVideo(httptest.NewRecorder(), req)

		assert.True(t, js.PublishCalled)
	})
}
