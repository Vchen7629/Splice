package test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
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
