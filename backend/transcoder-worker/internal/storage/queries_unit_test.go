//go:build unit

package storage_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"transcoder-worker/internal/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveTranscodedVideoChunkFileErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name        string
		filePath    string
		errContains string
	}{
		{
			name:        "nonexistent file returns error",
			filePath:    "/nonexistent/path/chunk.mp4",
			errContains: "error opening transcoded video file",
		},
		{
			name:        "directory instead of file returns error",
			filePath:    t.TempDir(),
			errContains: "error connecting to seaweedfs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url, err := storage.SaveTranscodedVideoChunk(srv.URL, tc.filePath, "job-123")

			require.Error(t, err)
			assert.Empty(t, url)
			assert.Contains(t, err.Error(), tc.errContains)
		})
	}
}

func TestSaveTranscodedVideoChunkHTTPErrors(t *testing.T) {
	validFile := filepath.Join(t.TempDir(), "chunk.mp4")
	require.NoError(t, os.WriteFile(validFile, []byte("fake video"), 0644))

	tests := []struct {
		name        string
		status      int
		wantErr     bool
		errContains string
	}{
		{
			name:        "500 returns error",
			status:      http.StatusInternalServerError,
			wantErr:     true,
			errContains: "seaweedfs upload failed",
		},
		{
			name:        "403 returns error",
			status:      http.StatusForbidden,
			wantErr:     true,
			errContains: "seaweedfs upload failed",
		},
		{
			name:    "200 returns url and no error",
			status:  http.StatusOK,
			wantErr: false,
		},
		{
			name:    "201 returns url and no error",
			status:  http.StatusCreated,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(srv.Close)

			url, err := storage.SaveTranscodedVideoChunk(srv.URL, validFile, "job-123")

			if tc.wantErr {
				require.Error(t, err)
				assert.Empty(t, url)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, url)
			}
		})
	}
}

func TestGetUnprocessedVideoChunkHTTPErrors(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		errContains string
	}{
		{
			name:        "404 returns video not found error",
			status:      http.StatusNotFound,
			errContains: "video not found",
		},
		{
			name:        "403 returns access denied error",
			status:      http.StatusForbidden,
			errContains: "access denied",
		},
		{
			name:        "500 returns error",
			status:      http.StatusInternalServerError,
			errContains: "error accessing seedweedfs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(srv.Close)

			jobID := "job-123"
			filePath, err := storage.GetUnprocessedVideoChunk(srv.URL+"/"+jobID+"/chunk.mp4", jobID)

			require.Error(t, err)
			assert.Empty(t, filePath)
			assert.Contains(t, err.Error(), tc.errContains)

			t.Cleanup(func() { os.RemoveAll("/tmp/temp-unprocessed-" + jobID) })
		})
	}
}

func TestGetUnprocessedVideoChunkWritesFile(t *testing.T) {
	videoContent := []byte("fake video content")
	jobID := "job-write"
	filename := "chunk_001.mp4"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(videoContent)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { os.RemoveAll("/tmp/temp-unprocessed-" + jobID) })

	storageURL := srv.URL + "/" + jobID + "/" + filename

	filePath, err := storage.GetUnprocessedVideoChunk(storageURL, jobID)

	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(filePath, filename), "filePath %q should end with %q", filePath, filename)
	assert.DirExists(t, "/tmp/temp-unprocessed-"+jobID)
	assert.FileExists(t, filePath)

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, videoContent, got)
}
