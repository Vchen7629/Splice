//go:build unit

package handler_test

import (
	"net/http"
	"shared/handler"
	"shared/test"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// server stops accepting connections after shutdown
func TestShutdownHttpServer(t *testing.T) {
	t.Skip()
	port := test.FreePort(t)
	server := handler.StartHttpServer(test.SilentLogger(), port)
	time.Sleep(50 * time.Millisecond)

	handler.ShutdownHttpServer(server, test.SilentLogger())

	_, err := http.Get("http://localhost:" + port + "/health")
	assert.Error(t, err, "server should no longer accept connections after shutdown")
}