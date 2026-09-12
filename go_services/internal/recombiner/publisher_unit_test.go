//go:build unit

package recombiner

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"splice.com/go_services/internal/shared/handler"
	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadVideoChunk(t *testing.T) {
	t.Cleanup(func() { removeAll = os.RemoveAll })

	t.Run("successful upload returns true and does not nak or clean up", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		outputPath := filepath.Join(t.TempDir(), "chunk.mp4")
		require.NoError(t, os.WriteFile(outputPath, []byte("fake video"), 0644))

		var removed []string
		removeAll = func(path string) error { removed = append(removed, path); return nil }

		msg := &test.MockMsg{}
		payload := handler.ChunkCompleteMessage{JobID: "job-1"}

		ok := uploadVideoChunk(outputPath, srv.URL, msg, payload, test.SilentLogger())

		assert.True(t, ok)
		assert.False(t, msg.NakCalled)
		assert.Empty(t, removed, "cleanup should be left to the caller on success")
	})

	t.Run("upload failure naks msg and cleans up temp folders", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		outputPath := filepath.Join(t.TempDir(), "chunk.mp4")
		require.NoError(t, os.WriteFile(outputPath, []byte("fake video"), 0644))

		var removed []string
		removeAll = func(path string) error { removed = append(removed, path); return nil }

		msg := &test.MockMsg{}
		payload := handler.ChunkCompleteMessage{JobID: "job-1"}

		ok := uploadVideoChunk(outputPath, srv.URL, msg, payload, test.SilentLogger())

		assert.False(t, ok)
		assert.True(t, msg.NakCalled)
		assert.Contains(t, removed, "/tmp/processed_chunk-job-1")
		assert.Contains(t, removed, "/tmp/jobs/job-1")
	})

	t.Run("missing output file naks msg and cleans up", func(t *testing.T) {
		var removed []string
		removeAll = func(path string) error { removed = append(removed, path); return nil }

		msg := &test.MockMsg{}
		payload := handler.ChunkCompleteMessage{JobID: "job-2"}

		ok := uploadVideoChunk("/nonexistent/chunk.mp4", "http://unused", msg, payload, test.SilentLogger())

		assert.False(t, ok)
		assert.True(t, msg.NakCalled)
		assert.NotEmpty(t, removed)
	})
}

func TestPublishJetstreamCompleteMsg(t *testing.T) {
	t.Cleanup(func() { removeAll = os.RemoveAll })

	payload := handler.ChunkCompleteMessage{JobID: "job-1", ChunkIndex: 0}

	t.Run("publish failure naks msg and does not write kv or ack", func(t *testing.T) {
		js := &test.MockJS{PublishErr: errors.New("nats unavailable")}
		msg := &test.MockMsg{}
		kv := &test.MockKV{}

		ok := publishJetstreamCompleteMsg(js, kv, msg, payload, test.SilentLogger())

		assert.False(t, ok)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.AckCalled)
		assert.Empty(t, kv.PutKey)
	})

	t.Run("kv write failure naks msg after publish succeeds", func(t *testing.T) {
		js := &test.MockJS{}
		msg := &test.MockMsg{}
		kv := &test.MockKV{PutErr: errors.New("kv unavailable")}

		ok := publishJetstreamCompleteMsg(js, kv, msg, payload, test.SilentLogger())

		assert.False(t, ok)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.AckCalled)
	})

	t.Run("success acks msg, writes kv, and cleans up temp folders", func(t *testing.T) {
		js := &test.MockJS{}
		msg := &test.MockMsg{}
		kv := &test.MockKV{}

		var removed []string
		removeAll = func(path string) error { removed = append(removed, path); return nil }

		ok := publishJetstreamCompleteMsg(js, kv, msg, payload, test.SilentLogger())

		assert.True(t, ok)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
		assert.Equal(t, "job-1.0", kv.PutKey)
		assert.Contains(t, removed, "/tmp/processed_chunk-job-1")
		assert.Contains(t, removed, "/tmp/jobs/job-1")
	})
}
