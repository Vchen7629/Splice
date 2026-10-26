//go:build unit

package service_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/service"
	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// health endpoint returns healthy status
func TestStartHealthHttpServer(t *testing.T) {
	port := test.FreePort(t)
	server := service.StartHealthHttpServer(test.SilentLogger(), port)
	t.Cleanup(func() { service.ShutdownHttpServer(server, test.SilentLogger()) })

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://localhost:" + port + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "Healthy", body["status"])
}

// server stops accepting connections after shutdown
func TestShutdownHttpServer(t *testing.T) {
	t.Skip()
	port := test.FreePort(t)
	server := service.StartHealthHttpServer(test.SilentLogger(), port)
	time.Sleep(50 * time.Millisecond)

	service.ShutdownHttpServer(server, test.SilentLogger())

	_, err := http.Get("http://localhost:" + port + "/health")
	assert.Error(t, err, "server should no longer accept connections after shutdown")
}
