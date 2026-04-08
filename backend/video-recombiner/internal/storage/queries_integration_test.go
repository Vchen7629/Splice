//go:build integration

package storage_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"video-recombiner/internal/storage"
	"video-recombiner/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProcessedVideoChunkIntegration(t *testing.T) {
	t.Run("fetches video and writes to correct local path with matching content", func(t *testing.T) {
		jobID := "job-fetch"
		filename := "testvideo.mp4"
		videoContent, err := os.ReadFile("../test/testvideo.mp4")
		require.NoError(t, err)

		test.SeedProcessedVideo(t, sharedFilerUrl, jobID, filename, videoContent)
		storageURL := fmt.Sprintf("%s/%s/%s/processed", sharedFilerUrl, jobID, filename)
		t.Cleanup(func() { os.RemoveAll("/tmp/processed_chunk-" + jobID) })

		filePath, err := storage.GetProcessedVideoChunk(storageURL, jobID)

		require.NoError(t, err)
		assert.Equal(t, "/tmp/processed_chunk-"+jobID+"/"+filename, filePath)
		assert.FileExists(t, filePath)

		got, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, videoContent, got)
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		jobID := "job-missing"
		storageURL := sharedFilerUrl + "/" + jobID + "/nonexistent.mp4"
		t.Cleanup(func() { os.RemoveAll("/tmp/processed_chunk-" + jobID) })

		filePath, err := storage.GetProcessedVideoChunk(storageURL, jobID)

		require.Error(t, err)
		assert.Empty(t, filePath)
	})
}

func TestUploadRecombinedVideo(t *testing.T) {
	t.Run("uploads file properly", func(t *testing.T) {
		videoFile := test.OpenTestVideo(t)

		url, err := storage.UploadRecombinedVideo(sharedFilerUrl, videoFile.Name(), "job-upload")

		require.NoError(t, err)

		expectedURL := fmt.Sprintf("%s/job-upload/testvideo.mp4/processed", sharedFilerUrl)
		assert.Equal(t, expectedURL, url)

		resp, err := http.Get(url)
		require.NoError(t, err)
		defer func() {
			err := resp.Body.Close()

			require.NoError(t, err)
		}()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		expected, err := io.ReadAll(test.OpenTestVideo(t))
		require.NoError(t, err)
		assert.Equal(t, expected, body)
	})
}
