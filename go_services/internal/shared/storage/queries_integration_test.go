//go:build integration

package storage_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/storage"
	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadVideoIntegration(t *testing.T) {
	t.Run("fetches video and writes to correct local path with matching content", func(t *testing.T) {
		jobID := "job-fetch"
		filename := "testvideo.mp4"
		videoContent, err := os.ReadFile("../test/testvideo.mp4")
		require.NoError(t, err)

		test.SeedProcessedVideo(t, sharedFilerUrl, jobID, filename, videoContent)
		storageURL := fmt.Sprintf("%s/%s/processed/%s", sharedFilerUrl, jobID, filename)
		t.Cleanup(func() { os.RemoveAll("/tmp/temp-unprocessed-" + jobID) })

		filePath, err := storage.DownloadVideo(storageURL, jobID, 30*time.Second)

		require.NoError(t, err)
		assert.Equal(t, "/tmp/temp-unprocessed-"+jobID+"/"+filename, filePath)
		assert.FileExists(t, filePath)

		got, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, videoContent, got)
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		jobID := "job-missing"
		storageURL := fmt.Sprintf("%s/%s/processed/nonexistent.mp4", sharedFilerUrl, jobID)
		t.Cleanup(func() { os.RemoveAll("/tmp/temp-unprocessed-" + jobID) })

		filePath, err := storage.DownloadVideo(storageURL, jobID, 30*time.Second)

		require.Error(t, err)
		assert.Empty(t, filePath)
	})
}

func TestUploadFile(t *testing.T) {
	t.Run("uploads file properly", func(t *testing.T) {
		videoFile := test.OpenTestVideo(t, "../test/testvideo.mp4")
		fileName := filepath.Base(videoFile.Name())
		uploadURL := fmt.Sprintf("%s/job-upload/processed/%s", sharedFilerUrl, fileName)

		err := storage.UploadFile(uploadURL, videoFile.Name(), 30*time.Second)

		require.NoError(t, err)

		resp, err := http.Get(uploadURL)
		require.NoError(t, err)
		defer func() {
			err := resp.Body.Close()
			require.NoError(t, err)
		}()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expected, err := io.ReadAll(test.OpenTestVideo(t, "../test/testvideo.mp4"))
		require.NoError(t, err)
		assert.Equal(t, expected, body)
	})
}
