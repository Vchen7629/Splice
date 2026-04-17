//go:build unit

package handler_test

import (
	"encoding/json"
	"net/http"
	shanlder "shared/handler"
	"testing"
	"time"
	"video-recombiner/internal/handler"
	"video-recombiner/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// health endpoint returns healthy status
func TestStartHttpServer(t *testing.T) {
	port := test.FreePort(t)
	server := handler.StartHttpServer(test.SilentLogger(), port)
	t.Cleanup(func() { shanlder.ShutdownHttpServer(server, test.SilentLogger()) })

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://localhost:" + port + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "Healthy", body["status"])
}
