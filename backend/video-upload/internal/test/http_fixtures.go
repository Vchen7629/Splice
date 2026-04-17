package test

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
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
	err = l.Close()
	require.NoError(t, err)
	return port
}

// NewDownloadRequest builds a GET request with a JSON body containing job_id and file_name.
func NewDownloadRequest(t *testing.T, jobID, fileName string) *http.Request {
	t.Helper()
	body := fmt.Sprintf(`{"job_id":%q,"file_name":%q}`, jobID, fileName)
	req, err := http.NewRequest(http.MethodGet, "/jobs", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}
