//go:build unit

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/test"
)

func TestUploadVideoRouteU(t *testing.T) {
	patchProbeVideo(t)

	t.Run("Returns 400 when body is not a multipart form", func(t *testing.T) {
		h := newVideoHandler("http://localhost:1", &MockJS{}, nil)
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
		{"video field is missing", "", nil, "1080p", "missing video file"},
		{"target_resolution is missing", "video.mp4", []byte("data"), "", "missing target_resolution field"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newVideoHandler("http://localhost:1", &MockJS{}, nil)
			req := NewUploadRequest(t, "/jobs", tc.fileName, tc.content, tc.targetRes, "transcode")
			rec := httptest.NewRecorder()

			h.uploadVideoRoute(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantMsg)
		})
	}

	t.Run("Returns 500 when saving the video file fails", func(t *testing.T) {
		// Null byte in the storage URL makes the upload request fail to build.
		h := newVideoHandler("\x00", &MockJS{}, nil)
		req := NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p", "upscale")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to process upload")
	})

	probeCases := []struct {
		name     string
		probe    func(context.Context, string) (videoInfo, error)
		wantCode int
		wantMsg  string
	}{
		{"returns 422 when probe rejects the video", func(context.Context, string) (videoInfo, error) {
			return videoInfo{}, &rejectError{"unreadable video"}
		}, http.StatusUnprocessableEntity, "unreadable video"},
		{"returns 500 when probe fails to run", func(context.Context, string) (videoInfo, error) {
			return videoInfo{}, errors.New("ffprobe missing")
		}, http.StatusInternalServerError, "failed to extract video info"},
		{"returns 422 when probed video fails validation", func(context.Context, string) (videoInfo, error) {
			return videoInfo{format: "flv", width: 64, height: 64, packets: 1}, nil
		}, http.StatusUnprocessableEntity, "unsupported video format"},
	}
	for _, tc := range probeCases {
		t.Run(tc.name, func(t *testing.T) {
			orig := probeVideoFn
			probeVideoFn = tc.probe
			t.Cleanup(func() { probeVideoFn = orig })
			js := &MockJS{}
			h := newVideoHandler("http://localhost:1", js, nil)
			req := NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p", "transcode")
			rec := httptest.NewRecorder()

			h.uploadVideoRoute(rec, req)

			assert.Equal(t, tc.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantMsg)
			assert.False(t, js.PublishCalled)
		})
	}

	t.Run("returns 500 when KV.Put fails during upload", func(t *testing.T) {
		fakeStorage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		t.Cleanup(fakeStorage.Close)

		h := newVideoHandler(fakeStorage.URL, &MockJS{}, nil)
		h.kv = &MockKV{PutErr: errors.New("kv unavailable")}

		req := NewUploadRequest(t, "/jobs/upload", "video.mp4", []byte("data"), "1080p", "transcode")
		w := httptest.NewRecorder()
		h.uploadVideoRoute(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to record job status")
	})

	t.Run("Does not publish to NATS when saving fails", func(t *testing.T) {
		js := &MockJS{}
		h := newVideoHandler("\x00", js, nil)
		req := NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p", "upscale")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.False(t, js.PublishCalled, "publish should not be called when save fails")
	})

	t.Run("returns 503 with Retry-After when upload concurrency limit reached", func(t *testing.T) {
		patchUploadAcquireTimeout(t)
		sem := make(chan struct{}, 1)
		sem <- struct{}{}
		h := newVideoHandler("http://localhost:1", &MockJS{}, sem)
		req := NewUploadRequest(t, "/jobs/upload", "video.mp4", []byte("data"), "1080p", "transcode")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Equal(t, "30", rec.Header().Get("Retry-After"))
		assert.Len(t, sem, 1) // rejected request must not have released someone else's slot
	})

	t.Run("slot is released regardless", func(t *testing.T) {
		sem := make(chan struct{}, 1)
		h := newVideoHandler("http://localhost:1", &MockJS{}, sem)
		req := NewUploadRequest(t, "/jobs/upload", "video.mp4", []byte("data"), "", "transcode")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Len(t, sem, 0)
	})

	t.Run("Rejects uploads exceeding MaxUploadBytes", func(t *testing.T) {
		h := &videoHandler{test.SilentLogger(), &test.MockJS{}, &test.MockKV{}, "", nil, 100}
		defer func() { h.maxUploadBytes = 0 }()

		req := NewUploadRequest(t, "/jobs", "big.mp4", bytes.Repeat([]byte("x"), 200), "1080p", "transcode")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid multipart form")
	})

	t.Run("Returns 500 when NATS publish fails after successful storage save", func(t *testing.T) {
		storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		t.Cleanup(storage.Close)

		js := &MockJS{PublishErr: errors.New("nats unavailab;e")}
		h := newVideoHandler(storage.URL, js, nil)
		req := NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p", "upscale")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to process upload")
		assert.True(t, js.PublishCalled, "should fail at publish, not before it")
	})

	t.Run("Uploads the exact bytes to storage, including uploads over the 32MB multipart memory limit", func(t *testing.T) {
		for _, size := range []int{1 << 10, 40 << 20} { // 40MB makes ParseMultipartForm spill to a temp file on disk
			var stored []byte
			var storedPath string
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				storedPath = r.URL.Path
				stored, _ = io.ReadAll(r.Body)
			}))
			t.Cleanup(storage.Close)

			h := newVideoHandler(storage.URL, &MockJS{}, nil)
			content := bytes.Repeat([]byte("x"), size)
			req := NewUploadRequest(t, "/jobs", "video.mp4", content, "1080p", "upscale")
			rec := httptest.NewRecorder()

			h.uploadVideoRoute(rec, req)

			require.Equal(t, http.StatusCreated, rec.Code)
			var resp uploadResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, "/"+resp.JobID+"/video.mp4", storedPath)
			assert.Equal(t, len(content), len(stored))
			assert.True(t, bytes.Equal(content, stored), "stored bytes differ for %d byte upload", size)
		}
	})
}

func TestAcquireUploadSlot(t *testing.T) {
	t.Run("acquires a free slot and release frees it", func(t *testing.T) {
		sem := make(chan struct{}, 1)
		h := newVideoHandler("", &MockJS{}, sem)

		release, ok := h.acquireUploadSlot(context.Background())
		require.True(t, ok)

		release()
		assert.Len(t, sem, 0)
	})

	t.Run("returns false when full and timeout elapses", func(t *testing.T) {
		patchUploadAcquireTimeout(t)
		sem := make(chan struct{}, 1)
		sem <- struct{}{}
		h := newVideoHandler("", &MockJS{}, sem)

		_, ok := h.acquireUploadSlot(context.Background())
		assert.False(t, ok)
	})

	t.Run("returns false when client cancels while waiting", func(t *testing.T) {
		sem := make(chan struct{}, 1)
		sem <- struct{}{}
		h := newVideoHandler("", &MockJS{}, sem)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, ok := h.acquireUploadSlot(ctx)
		assert.False(t, ok)
	})
}
