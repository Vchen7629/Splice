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
	t.Run("successful upload returns true and the storage url", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		outputPath := filepath.Join(t.TempDir(), "chunk.mp4")
		require.NoError(t, os.WriteFile(outputPath, []byte("fake video"), 0644))

		msg := &test.MockMsg{}

		ok, storageUrl := uploadVideoChunk(msg, outputPath, srv.URL, "job-1", test.SilentLogger())

		assert.True(t, ok)
		assert.NotEmpty(t, storageUrl)
		assert.False(t, msg.NakCalled)
	})

	t.Run("upload failure naks msg and returns empty storage url", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		outputPath := filepath.Join(t.TempDir(), "chunk.mp4")
		require.NoError(t, os.WriteFile(outputPath, []byte("fake video"), 0644))

		msg := &test.MockMsg{}

		ok, storageUrl := uploadVideoChunk(msg, outputPath, srv.URL, "job-1", test.SilentLogger())

		assert.False(t, ok)
		assert.Empty(t, storageUrl)
		assert.True(t, msg.NakCalled)
	})

	t.Run("missing output file naks msg and returns empty storage url", func(t *testing.T) {
		msg := &test.MockMsg{}

		ok, storageUrl := uploadVideoChunk(msg, "/nonexistent/chunk.mp4", "http://unused", "job-2", test.SilentLogger())

		assert.False(t, ok)
		assert.Empty(t, storageUrl)
		assert.True(t, msg.NakCalled)
	})
}

func TestPublishJetstreamProcessedMsg(t *testing.T) {
	payload := VideoChunkMessage{JobID: "job-1", ChunkIndex: 0, TotalChunks: 4}

	t.Run("publish failure naks msg and does not write kv or ack", func(t *testing.T) {
		js := &test.MockJetStream{PublishErr: errors.New("nats unavailable")}
		msg := &test.MockMsg{}
		kv := &test.MockKV{}
		pub := &mockPublisher{}

		ok := publishJetstreamProcessedMsg(pub, js, kv, msg, payload, "http://storage/chunk.mp4", test.SilentLogger())

		assert.False(t, ok)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.AckCalled)
		assert.Empty(t, kv.PutKey)
	})

	t.Run("kv write failure naks msg after publish succeeds and does not ack", func(t *testing.T) {
		js := &test.MockJetStream{}
		msg := &test.MockMsg{}
		kv := &test.MockKV{PutErr: errors.New("kv unavailable")}
		pub := &mockPublisher{}

		ok := publishJetstreamProcessedMsg(pub, js, kv, msg, payload, "http://storage/chunk.mp4", test.SilentLogger())

		assert.False(t, ok)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.AckCalled)
	})

	t.Run("success acks msg, writes kv, and reports progress", func(t *testing.T) {
		js := &test.MockJetStream{}
		msg := &test.MockMsg{}
		kv := &test.MockKV{}
		pub := &mockPublisher{}

		ok := publishJetstreamProcessedMsg(pub, js, kv, msg, payload, "http://storage/chunk.mp4", test.SilentLogger())

		assert.True(t, ok)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
		assert.Equal(t, "job-1.0", kv.PutKey)
		assert.Contains(t, pub.published, "progress.job-1")
	})

	t.Run("ack failure is logged but still returns true", func(t *testing.T) {
		js := &test.MockJetStream{}
		msg := &test.MockMsg{AckErr: errors.New("ack failed")}
		kv := &test.MockKV{}
		pub := &mockPublisher{}

		ok := publishJetstreamProcessedMsg(pub, js, kv, msg, payload, "http://storage/chunk.mp4", test.SilentLogger())

		assert.True(t, ok)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
	})
}
