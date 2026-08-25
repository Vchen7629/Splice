//go:build unit

package gateway

import (
	"net/http"
	"net/http/httptest"
	stest "splice.com/go_services/internal/shared/test"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForStage(t *testing.T) {
	urls := ServiceURLs{
		SceneDetector:  "http://scene:9098",
		Transcoder:     "http://transcoder:9095",
		Recombiner:     "http://recombiner:9090",
		VideoUpscaling: "http://upscaling:9101",
	}

	tests := []struct {
		stage   string
		wantURL string
		wantOK  bool
	}{
		{"scene-detector", "http://scene:9098", true},
		{"transcoder", "http://transcoder:9095", true},
		{"video-recombiner", "http://recombiner:9090", true},
		{"video-upscaling", "http://upscaling:9101", true},
		{"upload", "", false},
		{"unknown", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.stage, func(t *testing.T) {
			url, ok := urls.forStage(tc.stage)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantURL, url)
		})
	}

	t.Run("empty URL returns false", func(t *testing.T) {
		url, ok := ServiceURLs{}.forStage("scene-detector")
		assert.False(t, ok)
		assert.Empty(t, url)
	})
}

func TestIsServiceHealthy(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    bool
	}{
		{
			name:    "200 response returns true",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) },
			want:    true,
		},
		{
			name:    "503 response returns false",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
			want:    false,
		},
		{
			name:    "500 response returns false",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			assert.Equal(t, tc.want, isServiceHealthy(srv.URL, stest.SilentLogger()))
		})
	}

	t.Run("connection refused returns false", func(t *testing.T) {
		assert.False(t, isServiceHealthy("http://localhost:19999", stest.SilentLogger()))
	})
}
