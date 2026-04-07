//go:build integration

package storage_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"video-upload/internal/storage"
	"video-upload/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveUploadedVideo(t *testing.T, filerURL string) {
	t.Run("returns empty result and error", func(t *testing.T) {
		tests := []struct {
			name       string
			storageURL string
			handler    http.HandlerFunc
		}{
			{
				name:       "when storage is unreachable",
				storageURL: "http://localhost:1",
			},
			{
				name: "when storage returns status 400",
				handler: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
				},
			},
			{
				name: "when storage returns status 500",
				handler: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				url := tc.storageURL
				if tc.handler != nil {
					server := httptest.NewServer(tc.handler)
					defer server.Close()
					url = server.URL
				}

				result, err := storage.SaveUploadedVideo(test.OpenTestVideo(t), url, "testvideo.mp4")

				assert.Error(t, err)
				assert.Empty(t, result.JobID)
				assert.Empty(t, result.StorageURL)
			})
		}
	})

	t.Run("posts to correct url location", func(t *testing.T) {
		const fileName = "testvideo.mp4"
		var capturedPath string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		result, err := storage.SaveUploadedVideo(test.OpenTestVideo(t), server.URL, fileName)

		require.NoError(t, err)
		require.NotEmpty(t, result.JobID)
		assert.Equal(t, "/"+result.JobID+"/"+fileName, capturedPath)
		assert.Equal(t, server.URL+"/"+result.JobID+"/"+fileName, result.StorageURL)
	})

	t.Run("stores correct video file content at storage location", func(t *testing.T) {
		const fileName = "testvideo.mp4"
		storageURL := test.SetupSeaweedFSFiler(t)

		expectedBytes, err := os.ReadFile("../test/testvideo.mp4")
		require.NoError(t, err)

		result, err := storage.SaveUploadedVideo(test.OpenTestVideo(t), storageURL, fileName)
		require.NoError(t, err)

		resp, err := http.Get(result.StorageURL)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Less(t, resp.StatusCode, 400)
		storedBytes, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, expectedBytes, storedBytes)
	})
}

func TestGetProcessedVideo(t *testing.T) {
	t.Run("returns nil and error", func(t *testing.T) {
		tests := []struct {
			name       string
			storageURL string
			handler    http.HandlerFunc
		}{
			{
				name:       "when storage is unreachable",
				storageURL: "http://localhost:1",
			},
			{
				name: "when storage returns status 400",
				handler: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
				},
			},
			{
				name: "when storage returns status 500",
				handler: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				url := tc.storageURL
				if tc.handler != nil {
					server := httptest.NewServer(tc.handler)
					defer server.Close()
					url = server.URL
				}

				body, err := storage.GetProcessedVideo(url, "job123", "testvideo.mp4")

				assert.Error(t, err)
				assert.Nil(t, body)
			})
		}
	})

	t.Run("fetches correct video content from storage", func(t *testing.T) {
		const fileName = "testvideo.mp4"
		storageURL := test.SetupSeaweedFSFiler(t)

		expectedBytes, err := os.ReadFile("../test/testvideo.mp4")
		require.NoError(t, err)

		// seed the video at the /processed path that GetProcessedVideo will fetch
		seedReq, err := http.NewRequest(http.MethodPut, storageURL+"/job123/"+fileName+"/processed", test.OpenTestVideo(t))
		require.NoError(t, err)
		seedReq.Header.Set("Content-Type", "application/octet-stream")
		seedResp, err := http.DefaultClient.Do(seedReq)
		require.NoError(t, err)
		seedResp.Body.Close()
		require.Less(t, seedResp.StatusCode, 400)

		body, err := storage.GetProcessedVideo(storageURL, "job123", fileName)
		require.NoError(t, err)
		defer body.Close()

		fetchedBytes, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.Equal(t, expectedBytes, fetchedBytes)
	})
}
