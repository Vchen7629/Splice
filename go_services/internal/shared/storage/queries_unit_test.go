//go:build unit

package storage

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadVideoChunkFileErrors(t *testing.T) {
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
			errContains: "error opening video file",
		},
		{
			name:        "directory instead of file returns error",
			filePath:    t.TempDir(),
			errContains: "error connecting to seaweedfs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url, err := UploadVideoChunk(srv.URL, tc.filePath)

			require.Error(t, err)
			assert.Empty(t, url)
			assert.Contains(t, err.Error(), tc.errContains)
		})
	}
}

func TestUploadVideoChunk(t *testing.T) {
	validFile := filepath.Join(t.TempDir(), "chunk.mp4")
	require.NoError(t, os.WriteFile(validFile, []byte("fake video"), 0644))

	t.Run("returns the upload url on success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))
		t.Cleanup(srv.Close)

		url, err := UploadVideoChunk(srv.URL, validFile)

		require.NoError(t, err)
		assert.Equal(t, srv.URL, url)
	})

	t.Run("returns empty url when the upload fails", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		url, err := UploadVideoChunk(srv.URL, validFile)

		require.Error(t, err)
		assert.Empty(t, url)
	})
}

func TestUpload(t *testing.T) {
	t.Run("status codes", func(t *testing.T) {
		tests := []struct {
			name        string
			status      int
			wantErr     bool
			errContains string
		}{
			{name: "500 returns error", status: http.StatusInternalServerError, wantErr: true, errContains: "seaweedfs upload failed"},
			{name: "403 returns error", status: http.StatusForbidden, wantErr: true, errContains: "seaweedfs upload failed"},
			{name: "400 returns error", status: http.StatusBadRequest, wantErr: true, errContains: "seaweedfs upload failed"},
			{name: "200 returns no error", status: http.StatusOK},
			{name: "201 returns no error", status: http.StatusCreated},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
				}))
				t.Cleanup(srv.Close)

				err := Upload(srv.URL, strings.NewReader("fake video"), time.Second)

				if tc.wantErr {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tc.errContains)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("unreachable storage returns error", func(t *testing.T) {
		err := Upload("http://localhost:1", strings.NewReader("fake video"), time.Second)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "error connecting to seaweedfs")
	})

	t.Run("sends a PUT with the body, path and content type", func(t *testing.T) {
		var method, path, contentType string
		var body []byte

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method, path, contentType = r.Method, r.URL.Path, r.Header.Get("Content-Type")
			body, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
		}))
		t.Cleanup(srv.Close)

		err := Upload(srv.URL+"/job-1/video.mp4", strings.NewReader("fake video"), time.Second)

		require.NoError(t, err)
		assert.Equal(t, http.MethodPut, method)
		assert.Equal(t, "/job-1/video.mp4", path)
		assert.Equal(t, "application/octet-stream", contentType)
		assert.Equal(t, "fake video", string(body))
	})

	t.Run("returns error when storage does not respond within the timeout", func(t *testing.T) {
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
		t.Cleanup(srv.Close)
		t.Cleanup(func() { close(release) })

		err := Upload(srv.URL, strings.NewReader("fake video"), 50*time.Millisecond)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "error connecting to seaweedfs")
	})

	t.Run("zero timeout does not time out a slow response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusCreated)
		}))
		t.Cleanup(srv.Close)

		err := Upload(srv.URL, strings.NewReader("fake video"), 0)

		require.NoError(t, err)
	})
}

func TestGetVideoChunkHTTPErrors(t *testing.T) {
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
			filePath, err := GetVideoChunk(srv.URL+"/"+jobID+"/processed/chunk.mp4", jobID)

			require.Error(t, err)
			assert.Empty(t, filePath)
			assert.Contains(t, err.Error(), tc.errContains)

			t.Cleanup(func() { os.RemoveAll("/tmp/" + jobID) })
		})
	}
}

func TestGetVideoChunkWritesFile(t *testing.T) {
	videoContent := []byte("fake video content")
	jobID := "job-write"
	filename := "chunk_001.mp4"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(videoContent)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { os.RemoveAll("/tmp/temp-unprocessed-" + jobID) })

	storageURL := srv.URL + "/" + jobID + "/processed/" + filename

	filePath, err := GetVideoChunk(storageURL, jobID)

	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(filePath, filename), "filePath %q should end with %q", filePath, filename)
	assert.DirExists(t, "/tmp/temp-unprocessed-"+jobID)
	assert.FileExists(t, filePath)

	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, videoContent, got)
}

func TestGetVideoChunkIoCopyError(t *testing.T) {
	t.Run("io.Copy failure cleans up job dir and returns error", func(t *testing.T) {
		jobID := "job-copy-err"
		t.Cleanup(func() { os.RemoveAll("/tmp/" + jobID) })

		// Hijack the connection and close it mid-response to force io.Copy to fail
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hj, ok := w.(http.Hijacker)
			require.True(t, ok)
			conn, _, err := hj.Hijack()
			require.NoError(t, err)
			conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\npartial"))
			conn.Close()
		}))
		t.Cleanup(srv.Close)

		_, err := GetVideoChunk(srv.URL+"/"+jobID+"/processed/chunk.mp4", jobID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "error writing video to file")
		assert.NoDirExists(t, "/tmp/"+jobID)
	})

	t.Run("io.Copy failure with removeAll error returns removeAll error", func(t *testing.T) {
		jobID := "job-copy-removall-err"
		t.Cleanup(func() {
			removeAll = os.RemoveAll
			os.RemoveAll("/tmp/" + jobID)
		})

		rmErr := errors.New("remove failed")
		removeAll = func(_ string) error { return rmErr }

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hj, ok := w.(http.Hijacker)
			require.True(t, ok)
			conn, _, err := hj.Hijack()
			require.NoError(t, err)
			conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\npartial"))
			conn.Close()
		}))
		t.Cleanup(srv.Close)

		_, err := GetVideoChunk(srv.URL+"/"+jobID+"/processed/chunk.mp4", jobID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "error writing video to file")
		assert.Contains(t, err.Error(), "error removing all files")
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF, "original copy failure reason must not be lost")
		assert.ErrorIs(t, err, rmErr, "cleanup failure reason must not be lost")
	})
}
