//go:build unit

package transcoder

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPublisher stubs handler.Publisher for progress-reporting assertions.
type mockPublisher struct {
	published []string
	err       error
}

func (m *mockPublisher) Publish(subj string, _ []byte) error {
	m.published = append(m.published, subj)
	return m.err
}

func TestUploadVideoChunk(t *testing.T) {
	t.Run("successful upload returns storage url and no error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		outputPath := filepath.Join(t.TempDir(), "chunk.mp4")
		require.NoError(t, os.WriteFile(outputPath, []byte("fake video"), 0644))

		storageUrl, err := uploadVideoChunk(outputPath, srv.URL, "job-1", test.SilentLogger())

		assert.NotEmpty(t, storageUrl)
		assert.Nil(t, err)
	})

	t.Run("upload failure returns empty storage url and error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		outputPath := filepath.Join(t.TempDir(), "chunk.mp4")
		require.NoError(t, os.WriteFile(outputPath, []byte("fake video"), 0644))

		storageUrl, err := uploadVideoChunk(outputPath, srv.URL, "job-1", test.SilentLogger())

		assert.Empty(t, storageUrl)
		assert.NotNil(t, err)
	})

	t.Run("missing output file returns empty storage url and err", func(t *testing.T) {
		storageUrl, err := uploadVideoChunk("/nonexistent/chunk.mp4", "http://unused", "job-2", test.SilentLogger())

		assert.Empty(t, storageUrl)
		assert.NotNil(t, err)
	})
}

func TestPublishJetstreamProcessedMsg(t *testing.T) {
	payload := VideoChunkMessage{JobID: "job-abc", ChunkIndex: 2, TotalChunks: 4}

	t.Run("publish failure does not write kv or ack and err", func(t *testing.T) {
		js := &test.MockJS{PublishErr: errors.New("nats unavailable")}
		kv := &test.MockKV{}
		pub := &mockPublisher{}

		err := publishJetstreamProcessedMsg(pub, js, kv, payload, "http://storage/chunk.mp4", test.SilentLogger())

		assert.NotNil(t, err)
		assert.Empty(t, kv.PutKey)
	})

	t.Run("kv write failure returns err after publish succeeds but kv put fails", func(t *testing.T) {
		js := &test.MockJS{}
		kv := &test.MockKV{PutErr: errors.New("kv unavailable")}
		pub := &mockPublisher{}

		err := publishJetstreamProcessedMsg(pub, js, kv, payload, "http://storage/chunk.mp4", test.SilentLogger())

		assert.NotNil(t, err)
	})

	t.Run("success returns no error, writes kv, and reports progress", func(t *testing.T) {
		js := &test.MockJS{}
		kv := &test.MockKV{}
		pub := &mockPublisher{}

		err := publishJetstreamProcessedMsg(pub, js, kv, payload, "http://storage/chunk.mp4", test.SilentLogger())

		assert.Nil(t, err)
		assert.Equal(t, "job-abc.2", kv.PutKey)
		assert.Contains(t, pub.published, "progress.job-abc")
	})
}
