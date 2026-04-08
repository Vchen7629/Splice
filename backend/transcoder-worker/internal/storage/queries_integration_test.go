//go:build integration

package storage_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"transcoder-worker/internal/storage"
	"transcoder-worker/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUnprocessedVideoChunkIntegration(t *testing.T) {
	t.Run("fetches video and writes to correct local path with matching content", func(t *testing.T) {
		jobID := "job-fetch"
		filename := "test_video.mp4"
		videoContent, err := os.ReadFile("../test/test_video.mp4")
		require.NoError(t, err)

		storageURL := test.SeedUnprocessedVideo(t, sharedFilerUrl, jobID, filename, videoContent)
		t.Cleanup(func() { os.RemoveAll("/tmp/temp-unprocessed-" + jobID) })

		filePath, err := storage.GetUnprocessedVideoChunk(storageURL, jobID)

		require.NoError(t, err)
		assert.Equal(t, "/tmp/temp-unprocessed-"+jobID+"/"+filename, filePath)
		assert.FileExists(t, filePath)

		got, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, videoContent, got)
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		jobID := "job-missing"
		storageURL := sharedFilerUrl + "/" + jobID + "/nonexistent.mp4"
		t.Cleanup(func() { os.RemoveAll("/tmp/temp-unprocessed-" + jobID) })

		filePath, err := storage.GetUnprocessedVideoChunk(storageURL, jobID)

		require.Error(t, err)
		assert.Empty(t, filePath)
	})
}

func TestSaveTranscodedVideoChunk(t *testing.T) {
	t.Run("uploads file properly", func(t *testing.T) {
		videoFile := test.OpenTestVideo(t)

		url, err := storage.SaveTranscodedVideoChunk(sharedFilerUrl, videoFile.Name(), "job-upload")

		require.NoError(t, err)

		expectedURL := fmt.Sprintf("%s/job-upload/processed/test_video.mp4", sharedFilerUrl)
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
