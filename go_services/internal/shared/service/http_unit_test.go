//go:build unit

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"splice.com/go_services/internal/shared/test"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// health endpoint returns healthy status
func TestHealthRoute(t *testing.T) {
	mux := newMux(prometheus.NewRegistry(), test.SilentLogger())

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	assert.Equal(t, "Healthy", body["status"])
}

func TestMetricsRoute(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	mux := newMux(reg, test.SilentLogger())

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "go_goroutines")
}

// non-GET on /health is rejected by the method-scoped route
func TestRouteRejectsPost(t *testing.T) {
	mux := newMux(prometheus.NewRegistry(), test.SilentLogger())

	for _, endpoint := range []string{"/health", "/metrics"} {
		t.Run(endpoint, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, endpoint, nil))

			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}
