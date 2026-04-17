//go:build integration

package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"shared/handler"
	"testing"
	"time"
	"video-upload/internal/service"
	"video-upload/internal/test"

	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sharedFilerUrl string

func TestMain(m *testing.M) {
	filerURL, cleanup := test.StartSeaweedFSFiler()
	sharedFilerUrl = filerURL

	code := m.Run()

	cleanup()
	os.Exit(code)
}

func newUploadHandler(js jetstream.JetStream, kv jetstream.KeyValue, filerURL string) *videoHandler {
	return &videoHandler{
		logger:     test.SilentLogger(),
		js:         js,
		kv:         kv,
		storageURL: filerURL,
	}
}

func newDownloadVideoServer(t *testing.T, storageURL string) *httptest.Server {
	t.Helper()
	h := &videoHandler{
		logger:     test.SilentLogger(),
		storageURL: storageURL,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /jobs", h.downloadVideoRoute)
	return httptest.NewServer(mux)
}

// covers the full upload pipeline: multipart form → SeaweedFS → NATS → response.
func TestUploadVideoFlow(t *testing.T) {
	js, nc := test.SetupNats(t)
	kv := test.SetupKV(t, js)
	h := newUploadHandler(js, kv, sharedFilerUrl)

	t.Run("Rejects uploads exceeding MaxUploadBytes", func(t *testing.T) {
		h.maxUploadBytes = 100
		defer func() { h.maxUploadBytes = 0 }()

		req := test.NewUploadRequest(t, "/jobs", "big.mp4", bytes.Repeat([]byte("x"), 200), "1080p")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid multipart form")
	})

	t.Run("File is saved to SeaweedFS and is fetchable at the returned StorageURL", func(t *testing.T) {
		req := test.NewUploadRequest(t, "/jobs", "clip.mp4", test.TestVideoBytes(t), "1080p")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)
		var resp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.NotEmpty(t, resp.JobID)
	})

	t.Run("Saved file contains the exact bytes that were uploaded", func(t *testing.T) {
		content := test.TestVideoBytes(t)
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", content, "720p")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)
		var resp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

		stored, err := http.Get(fmt.Sprintf("%s/%s/video.mp4", sharedFilerUrl, resp.JobID))
		require.NoError(t, err)
		defer stored.Body.Close()
		require.Less(t, stored.StatusCode, 400)
		storedBytes, err := io.ReadAll(stored.Body)
		require.NoError(t, err)
		assert.Equal(t, content, storedBytes)
	})

	t.Run("Published NATS message contains correct job_id, target_resolution, and storage_url", func(t *testing.T) {
		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.video.scene-split", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		defer sub.Unsubscribe()

		req := test.NewUploadRequest(t, "/jobs", "video.mp4", test.TestVideoBytes(t), "720p")
		rec := httptest.NewRecorder()
		h.uploadVideoRoute(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var uploadResp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&uploadResp))

		select {
		case data := <-received:
			var msg handler.VideoJobMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, uploadResp.JobID, msg.JobID)
			assert.Equal(t, "720p", msg.TargetResolution)
			assert.Equal(t, fmt.Sprintf("%s/%s/video.mp4", sharedFilerUrl, uploadResp.JobID), msg.StorageURL)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for NATS message")
		}
	})

	t.Run("Multiple uploads produce unique job IDs", func(t *testing.T) {
		seen := make(map[string]bool)

		for range 3 {
			req := test.NewUploadRequest(t, "/jobs", "video.mp4", test.TestVideoBytes(t), "1080p")
			rec := httptest.NewRecorder()
			h.uploadVideoRoute(rec, req)
			require.Equal(t, http.StatusCreated, rec.Code)

			var resp struct {
				JobID string `json:"job_id"`
			}
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.False(t, seen[resp.JobID], "duplicate job_id: %s", resp.JobID)
			seen[resp.JobID] = true
		}
	})

	t.Run("Large file (5 MB) is fully persisted to SeaweedFS", func(t *testing.T) {
		checkVideoResolution = func(_ multipart.File) (string, error) { return "1080p", nil }
		t.Cleanup(func() { checkVideoResolution = service.CheckVideoResolution })

		content := bytes.Repeat([]byte("x"), 5*1024*1024)
		req := test.NewUploadRequest(t, "/jobs", "big.mp4", content, "4k")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)
		var resp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

		stored, err := http.Get(fmt.Sprintf("%s/%s/big.mp4", sharedFilerUrl, resp.JobID))
		require.NoError(t, err)
		defer stored.Body.Close()
		require.Less(t, stored.StatusCode, 400)
		storedBytes, err := io.ReadAll(stored.Body)
		require.NoError(t, err)
		assert.Equal(t, len(content), len(storedBytes))
	})

	t.Run("Returns 500 when NATS publish fails after successful storage save", func(t *testing.T) {
		checkVideoResolution = func(_ multipart.File) (string, error) { return "1080p", nil }
		t.Cleanup(func() { checkVideoResolution = service.CheckVideoResolution })

		h := newUploadHandler(&test.MockJS{PublishErr: errors.New("nats unavailable")}, &test.MockKV{}, sharedFilerUrl)
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
		rec := httptest.NewRecorder()

		h.uploadVideoRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "unable to send process request")
	})
}

func TestDownloadVideoFlow(t *testing.T) {
	ts := newDownloadVideoServer(t, sharedFilerUrl)
	defer ts.Close()

	t.Run("Streams the exact bytes of a seeded processed video", func(t *testing.T) {
		content := []byte("fake processed video bytes")
		test.SeedProcessedVideo(t, sharedFilerUrl, "job-1", "output.mp4", content)

		req, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs", test.NewDownloadRequest(t, "job-1", "output.mp4").Body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, content, body)
	})

	t.Run("Returns correct Content-Disposition and Content-Type headers", func(t *testing.T) {
		test.SeedProcessedVideo(t, sharedFilerUrl, "job-2", "output.mp4", []byte("data"))

		req, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs", test.NewDownloadRequest(t, "job-2", "output.mp4").Body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, "application/octet-stream", resp.Header.Get("Content-Type"))
		assert.Contains(t, resp.Header.Get("Content-Disposition"), "output.mp4")
	})

	t.Run("Returns 500 when the processed video does not exist in storage", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs", test.NewDownloadRequest(t, "no-such-job", "output.mp4").Body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}
