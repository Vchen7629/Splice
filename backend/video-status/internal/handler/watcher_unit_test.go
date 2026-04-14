//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"video-status/internal/test"

	"github.com/stretchr/testify/assert"
)

func TestNextServiceMap(t *testing.T) {
	tests := []struct {
		stage string
		want  string
	}{
		{"upload", "scene-detector"},
		{"scene-detector", "transcoder"},
		{"transcoder", "video-recombiner"},
	}

	for _, tc := range tests {
		t.Run(tc.stage, func(t *testing.T) {
			got, ok := nextService[tc.stage]
			assert.True(t, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestForStage(t *testing.T) {
	urls := ServiceURLs{
		SceneDetector: "http://scene:9098",
		Transcoder:    "http://transcoder:9095",
		Recombiner:    "http://recombiner:9090",
	}

	tests := []struct {
		stage   string
		wantURL string
		wantOK  bool
	}{
		{"upload", "http://scene:9098", true},
		{"scene-detector", "http://transcoder:9095", true},
		{"transcoder", "http://recombiner:9090", true},
		{"video-recombine", "", false},
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
			assert.Equal(t, tc.want, isServiceHealthy(srv.URL, test.SilentLogger()))
		})
	}

	t.Run("connection refused returns false", func(t *testing.T) {
		assert.False(t, isServiceHealthy("http://localhost:19999", test.SilentLogger()))
	})
}

func TestCheckServiceHealth(t *testing.T) {
	healthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthySrv.Close()

	downURL := "http://localhost:19999"

	tests := []struct {
		name       string
		status     JobStatus
		urls       ServiceURLs
		wantState  JobState
		wantErrMsg string
	}{
		{
			name:      "PROCESSING with healthy service stays PROCESSING",
			status:    JobStatus{State: StateProcessing, Stage: "scene-detector"},
			urls:      ServiceURLs{Transcoder: healthySrv.URL},
			wantState: StateProcessing,
		},
		{
			name:       "PROCESSING with service down becomes DEGRADED",
			status:     JobStatus{State: StateProcessing, Stage: "scene-detector"},
			urls:       ServiceURLs{Transcoder: downURL},
			wantState:  StateDegraded,
			wantErrMsg: "service unavailable at stage: transcoder",
		},
		{
			name:      "DEGRADED with service recovered becomes PROCESSING and clears error",
			status:    JobStatus{State: StateDegraded, Stage: "scene-detector", Error: "service unavailable at stage: transcoder"},
			urls:      ServiceURLs{Transcoder: healthySrv.URL},
			wantState: StateProcessing,
		},
		{
			name:       "DEGRADED with service still down stays DEGRADED",
			status:     JobStatus{State: StateDegraded, Stage: "scene-detector"},
			urls:       ServiceURLs{Transcoder: downURL},
			wantState:  StateDegraded,
			wantErrMsg: "service unavailable at stage: transcoder",
		},
		{
			name:      "unknown stage returns status unchanged",
			status:    JobStatus{State: StateProcessing, Stage: "unknown-stage"},
			urls:      ServiceURLs{},
			wantState: StateProcessing,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := checkServiceHealth(tc.status, tc.urls, test.SilentLogger())
			assert.Equal(t, tc.wantState, result.State)
			assert.Equal(t, tc.wantErrMsg, result.Error)
		})
	}
}
