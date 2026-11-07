//go:build unit

package gateway

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewDownloadRequest builds a GET request with a JSON body containing job_id and file_name.
func NewDownloadRequest(t *testing.T, target, fileName string, jobID uuid.UUID) *http.Request {
	t.Helper()
	body := fmt.Sprintf(`{"job_id":%q,"file_name":%q}`, jobID, fileName)
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestDownloadVideoRouteU(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"invalid json", `not json`, "invalid json payload"},
		{"missing job_id", `{"file_name":"video.mp4"}`, ""},
		{"missing file_name", `{"job_id":"abc-123"}`, ""},
		{"job_id non uuid", `{"job_id":"a","file_name":"video.mp4"}`, ""},
		{"file_name too short", `{"job_id":"abc-123","file_name":"v"}`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newVideoHandler("http://localhost:1", &MockJS{}, nil)
			req := httptest.NewRequest(http.MethodGet, "/jobs", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			h.downloadVideoRoute(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			if tc.wantMsg != "" {
				assert.Contains(t, rec.Body.String(), tc.wantMsg)
			}
		})
	}

	t.Run("Returns 500 when storage is unreachable", func(t *testing.T) {
		h := newVideoHandler("http://localhost:1", &MockJS{}, nil)
		req := NewDownloadRequest(t, "/jobs/download", "video.mp4", uuid.New())
		rec := httptest.NewRecorder()

		h.downloadVideoRoute(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "failed to fetch video")
	})

	content := []byte("fake processed video bytes")
	id := uuid.New()

	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+id.String()+"/output.mp4/processed" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(content)
	}))
	t.Cleanup(storage.Close)
	h := newVideoHandler(storage.URL, &MockJS{}, nil)

	t.Run("Streams the exact bytes of a seeded processed video", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.downloadVideoRoute(rec, NewDownloadRequest(t, "/jobs/download", "output.mp4", id))

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, content, rec.Body.Bytes())
	})

	t.Run("Returns correct Content-Disposition and Content-Type headers", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.downloadVideoRoute(rec, NewDownloadRequest(t, "/jobs/download", "output.mp4", id))

		assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Header().Get("Content-Disposition"), "output.mp4")
	})

	t.Run("Returns 500 when the processed video does not exist in storage", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.downloadVideoRoute(rec, NewDownloadRequest(t, "/jobs/download", "output.mp4", uuid.New()))

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestGetProcessedVideoU(t *testing.T) {
	t.Run("returns nil and error", func(t *testing.T) {
		tests := []struct {
			name       string
			storageURL string
			handler    http.HandlerFunc
		}{
			{name: "when storage is unreachable", storageURL: "http://localhost:1"},
			{name: "when storage returns status 404", handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }},
			{name: "when storage returns status 403", handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }},
			{name: "when storage returns status 500", handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				url := tc.storageURL
				if tc.handler != nil {
					server := httptest.NewServer(tc.handler)
					defer server.Close()
					url = server.URL
				}

				body, err := getProcessedVideo(t.Context(), url, "job123", "testvideo.mp4")

				assert.Error(t, err)
				assert.Nil(t, body)
			})
		}
	})
}
