//go:build integration

package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
	"video-upload/internal/handler"
	"video-upload/internal/service"
	"video-upload/internal/test"

	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUploadHandler(t *testing.T) (*handler.VideoHandler, *nats.Conn, string) {
	t.Helper()
	js, nc := test.SetupNats(t)
	dir := t.TempDir()
	return &handler.VideoHandler{
		Logger:    test.SilentLogger(),
		JS:        js,
		OutputDir: dir,
	}, nc, dir
}

func newDownloadVideoServer(t *testing.T, outputDir string) *httptest.Server {
	t.Helper()
	h := &handler.VideoHandler{
		Logger:    test.SilentLogger(),
		OutputDir: outputDir,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /jobs/{id}", h.DownloadVideo)
	return httptest.NewServer(mux)
}

func writeOutputFile(t *testing.T, outputDir, jobID string, content []byte) {
	t.Helper()
	path := filepath.Join(outputDir, "jobs", jobID, "output.mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, content, 0644))
}

// covers the full upload pipeline: multipart form → disk → NATS → response.
func TestUploadVideoFlow(t *testing.T) {
	t.Run("Rejects uploads exceeding MaxUploadBytes", func(t *testing.T) {
		h, _, _ := newUploadHandler(t)
		h.MaxUploadBytes = 100 // 100 bytes

		// building a multipart body whose file part exceeds 100 bytes
		req := test.NewUploadRequest(t, "/jobs", "big.mp4", bytes.Repeat([]byte("x"), 200), "1080p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid multipart form")
	})
	t.Run("File is saved to disk at outputDir/jobs/jobID/filename", func(t *testing.T) {
		h, _, dir := newUploadHandler(t)
		req := test.NewUploadRequest(t, "/jobs", "clip.mp4", []byte("fake video bytes"), "1080p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)
		var resp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.FileExists(t, filepath.Join(dir, "jobs", resp.JobID, "clip.mp4"))
	})

	t.Run("Saved file contains the exact bytes that were uploaded", func(t *testing.T) {
		h, _, dir := newUploadHandler(t)
		content := []byte("precise video content")
		req := test.NewUploadRequest(t, "/jobs", "video.mp4", content, "720p")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)
		var resp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

		saved, err := os.ReadFile(filepath.Join(dir, "jobs", resp.JobID, "video.mp4"))
		require.NoError(t, err)
		assert.Equal(t, content, saved)
	})

	t.Run("Published NATS message contains correct job_id, target_resolution, and storage_path", func(t *testing.T) {
		h, nc, dir := newUploadHandler(t)

		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.video.scene-split", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "720p")
		rec := httptest.NewRecorder()
		h.UploadVideo(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)

		var uploadResp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&uploadResp))

		select {
		case data := <-received:
			var msg service.SceneSplitMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, uploadResp.JobID, msg.JobID)
			assert.Equal(t, "720p", msg.TargetResolution)
			assert.Equal(t, filepath.Join(dir, "jobs", uploadResp.JobID, "video.mp4"), msg.StoragePath)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for NATS message")
		}
	})

	t.Run("Multiple uploads save files to separate job directories", func(t *testing.T) {
		h, _, dir := newUploadHandler(t)
		seen := make(map[string]bool)

		for range 3 {
			req := test.NewUploadRequest(t, "/jobs", "video.mp4", []byte("data"), "1080p")
			rec := httptest.NewRecorder()
			h.UploadVideo(rec, req)
			require.Equal(t, http.StatusCreated, rec.Code)

			var resp struct {
				JobID string `json:"job_id"`
			}
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

			jobDir := filepath.Join(dir, "jobs", resp.JobID)
			assert.False(t, seen[jobDir], "duplicate job directory: %s", jobDir)
			seen[jobDir] = true
		}
	})

	t.Run("Large file (5 MB) is fully persisted to disk", func(t *testing.T) {
		h, _, dir := newUploadHandler(t)
		content := bytes.Repeat([]byte("x"), 5*1024*1024)
		req := test.NewUploadRequest(t, "/jobs", "big.mp4", content, "4k")
		rec := httptest.NewRecorder()

		h.UploadVideo(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)
		var resp struct {
			JobID string `json:"job_id"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

		saved, err := os.ReadFile(filepath.Join(dir, "jobs", resp.JobID, "big.mp4"))
		require.NoError(t, err)
		assert.Equal(t, len(content), len(saved))
	})
}

func TestDownloadVideoFlow(t *testing.T) {
	t.Run("Serves the complete file with correct body content", func(t *testing.T) {
		dir := t.TempDir()
		content := []byte("fake mp4 bytes")
		writeOutputFile(t, dir, "job-1", content)

		ts := newDownloadVideoServer(t, dir)
		defer ts.Close()

		resp, err := http.Get(fmt.Sprintf("%s/jobs/job-1", ts.URL))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, content, body)
	})

	t.Run("Response includes Accept-Ranges header", func(t *testing.T) {
		dir := t.TempDir()
		writeOutputFile(t, dir, "job-2", []byte("data"))

		ts := newDownloadVideoServer(t, dir)
		defer ts.Close()

		resp, err := http.Get(fmt.Sprintf("%s/jobs/job-2", ts.URL))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, "bytes", resp.Header.Get("Accept-Ranges"))
	})

	t.Run("Range request returns 206 Partial Content with the correct bytes", func(t *testing.T) {
		dir := t.TempDir()
		writeOutputFile(t, dir, "job-3", []byte("abcdefghij"))

		ts := newDownloadVideoServer(t, dir)
		defer ts.Close()

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/jobs/job-3", ts.URL), nil)
		require.NoError(t, err)
		req.Header.Set("Range", "bytes=2-5")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusPartialContent, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, []byte("cdef"), body)
	})

	t.Run("Conditional GET with If-Modified-Since returns 304 Not Modified", func(t *testing.T) {
		dir := t.TempDir()
		writeOutputFile(t, dir, "job-4", []byte("data"))

		ts := newDownloadVideoServer(t, dir)
		defer ts.Close()

		// http.ServeFile sets Last-Modified but not ETag — use If-Modified-Since for conditional GETs
		first, err := http.Get(fmt.Sprintf("%s/jobs/job-4", ts.URL))
		require.NoError(t, err)
		first.Body.Close()
		lastModified := first.Header.Get("Last-Modified")
		require.NotEmpty(t, lastModified, "http.ServeFile should set a Last-Modified header")

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/jobs/job-4", ts.URL), nil)
		require.NoError(t, err)
		req.Header.Set("If-Modified-Since", lastModified)

		second, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		second.Body.Close()

		assert.Equal(t, http.StatusNotModified, second.StatusCode)
	})

	t.Run("Missing output file returns 404", func(t *testing.T) {
		ts := newDownloadVideoServer(t, t.TempDir())
		defer ts.Close()

		resp, err := http.Get(fmt.Sprintf("%s/jobs/no-such-job", ts.URL))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
