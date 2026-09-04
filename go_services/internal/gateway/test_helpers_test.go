//go:build unit || integration

package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// Helpers used by two or more test files in this package live here.
// Anything used by a single file is defined inline in that file; anything
// used by another package lives in internal/shared/test.

// droppedConnectionWriter simulates a client disconnecting before the response write completes.
// Used by http_unit_test.go and http_integration_test.go.
type droppedConnectionWriter struct {
	header     http.Header
	statusCode int
}

func newDroppedConnectionWriter() *droppedConnectionWriter {
	return &droppedConnectionWriter{header: make(http.Header)}
}

func (d *droppedConnectionWriter) Header() http.Header  { return d.header }
func (d *droppedConnectionWriter) WriteHeader(code int) { d.statusCode = code }
func (d *droppedConnectionWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}

// NewUploadRequest builds a multipart POST request.
// Pass a path ("/jobs") for direct handler invocation via httptest.NewRecorder,
// or a full URL ("http://host:port/jobs") for use with http.DefaultClient.
// Used by upload_unit_test.go and upload_integration_test.go.
func NewUploadRequest(t *testing.T, target, filename string, fileContent []byte, targetRes, sourceRes, processType string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	if filename != "" {
		fw, err := w.CreateFormFile("video", filename)
		require.NoError(t, err)
		_, err = fw.Write(fileContent)
		require.NoError(t, err)
	}

	if targetRes != "" {
		require.NoError(t, w.WriteField("target_resolution", targetRes))
	}

	if sourceRes != "" {
		require.NoError(t, w.WriteField("source_resolution", sourceRes))
	}

	if processType != "" {
		require.NoError(t, w.WriteField("process_type", processType))
	}

	require.NoError(t, w.Close())

	req, err := http.NewRequest(http.MethodPost, target, &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// NewDownloadRequest builds a GET request with a JSON body containing job_id and file_name.
func NewDownloadRequest(t *testing.T, target, jobID, fileName string) *http.Request {
	t.Helper()
	body := fmt.Sprintf(`{"job_id":%q,"file_name":%q}`, jobID, fileName)
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// MockKV stubs jetstream.KeyValue.
// Seed pre-populates entries; GetErr forces an error on every Get; PutErr forces an error on every Put.
type MockKV struct {
	jetstream.KeyValue
	GetErr       error
	PutErr       error
	WatchErr     error
	UpdateErr    error
	PutCalled    atomic.Bool
	UpdateCalled atomic.Bool
	entries      map[string][]byte
}

func NewMockKV() *MockKV {
	return &MockKV{entries: make(map[string][]byte)}
}

// Seed stores a raw value under key so Get returns it.
func (m *MockKV) Seed(key string, value []byte) {
	if m.entries == nil {
		m.entries = make(map[string][]byte)
	}
	m.entries[key] = value
}

func (m *MockKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	v, ok := m.entries[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return &MockKVEntry{value: v, revision: 0}, nil
}

func (m *MockKV) Put(_ context.Context, _ string, _ []byte) (uint64, error) {
	m.PutCalled.Store(true)
	return 0, m.PutErr
}

func (m *MockKV) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	if m.WatchErr != nil {
		return nil, m.WatchErr
	}

	return &MockKeyWatcher{updates: make(chan jetstream.KeyValueEntry)}, nil
}

func (m *MockKV) Update(_ context.Context, _ string, _ []byte, _ uint64) (uint64, error) {
	m.UpdateCalled.Store(true)
	return 0, m.UpdateErr
}

// stubs jetstream.KeyWatcher. Only used to satisfy MockKV.Watch's success return type
type MockKeyWatcher struct {
	updates chan jetstream.KeyValueEntry
}

func (w *MockKeyWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *MockKeyWatcher) Stop() error                             { return nil }

// MockKVEntry stubs jetstream.KeyValueEntry.
type MockKVEntry struct {
	jetstream.KeyValueEntry
	value    []byte
	revision uint64
}

func (e *MockKVEntry) Value() []byte    { return e.value }
func (e *MockKVEntry) Revision() uint64 { return e.revision }

// MockJS stubs jetstream.JetStream.
// Used by subscriber_unit_test.go, upload_unit_test.go and upload_integration_test.go.
type MockJS struct {
	jetstream.JetStream
	JStreamNameErr error
	JStreamErr     error
	JStream        jetstream.Stream
	PublishErr     error
	PublishCalled  bool
}

func (m *MockJS) StreamNameBySubject(_ context.Context, _ string) (string, error) {
	return "jobs", m.JStreamNameErr
}

func (m *MockJS) Stream(_ context.Context, _ string) (jetstream.Stream, error) {
	return m.JStream, m.JStreamErr
}

func (m *MockJS) Publish(_ context.Context, _ string, _ []byte, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	m.PublishCalled = true
	return nil, m.PublishErr
}

// patchOsExit replaces the package osExit seam for the duration of the test.
// Used by http_unit_test.go (StartHttpApi's listen failure) and
// job_status_kv_integration_test.go (CreateJobStatusKV's bucket failure).
func patchOsExit(t *testing.T) *int {
	t.Helper()
	code := new(int)
	*code = -1
	osExit = func(c int) { *code = c }
	t.Cleanup(func() { osExit = os.Exit })
	return code
}
