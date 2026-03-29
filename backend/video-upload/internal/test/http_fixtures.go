package test

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"video-upload/internal/handler"
	"video-upload/internal/service"

	"github.com/stretchr/testify/require"
)

func SilentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// droppedConnectionWriter simulates a client disconnecting before the response write completes.
type droppedConnectionWriter struct {
	header     http.Header
	statusCode int
}

func NewDroppedConnectionWriter() *droppedConnectionWriter {
	return &droppedConnectionWriter{header: make(http.Header)}
}

func (d *droppedConnectionWriter) Header() http.Header  { return d.header }
func (d *droppedConnectionWriter) WriteHeader(code int) { d.statusCode = code }
func (d *droppedConnectionWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}

// http test server for http integration tests
// FreePort returns an OS-assigned free port number as a string.
// There is a small TOCTOU window between Close and the caller binding the port,
// but it is acceptable for tests.
func FreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	return port
}

func NewTestServer(tracker *service.CompletedJobs) *httptest.Server {
	h := &handler.JobStatusHandler{
		Logger:  slog.Default(),
		Tracker: tracker,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /jobs/{id}", h.PollJobStatus)
	return httptest.NewServer(mux)
}
