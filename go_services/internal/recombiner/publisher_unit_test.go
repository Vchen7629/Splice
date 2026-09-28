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
	logger := test.SilentLogger()
	t.Run("successful upload returns nil error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		outputPath := filepath.Join(t.TempDir(), "chunk.mp4")
		require.NoError(t, os.WriteFile(outputPath, []byte("fake video"), 0644))

		payload := handler.ChunkCompleteMessage{JobID: "job-1"}

		err := uploadVideoChunk(outputPath, srv.URL, payload, logger)

		assert.Nil(t, err)
	})

	t.Run("upload failure returns err", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		outputPath := filepath.Join(t.TempDir(), "chunk.mp4")
		require.NoError(t, os.WriteFile(outputPath, []byte("fake video"), 0644))

		payload := handler.ChunkCompleteMessage{JobID: "job-1"}

		err := uploadVideoChunk(outputPath, srv.URL, payload, logger)

		assert.NotNil(t, err)
	})

	t.Run("missing output file returns err", func(t *testing.T) {
		payload := handler.ChunkCompleteMessage{JobID: "job-2"}

		err := uploadVideoChunk("/nonexistent/chunk.mp4", "http://unused", payload, logger)

		assert.NotNil(t, err)
	})
}

func TestPublishJetstreamCompleteMsg(t *testing.T) {
	payload := handler.ChunkCompleteMessage{JobID: "job-1", ChunkIndex: 0}

	t.Run("publish failure returns the err", func(t *testing.T) {
		js := &test.MockJS{PublishErr: errors.New("nats unavailable")}
		kv := &test.MockKV{}

		err := publishJetstreamCompleteMsg(js, kv, payload, test.SilentLogger())

		assert.NotNil(t, err)
	})

	t.Run("kv write failure returns err after publish succeeds", func(t *testing.T) {
		js := &test.MockJS{}
		kv := &test.MockKV{PutErr: errors.New("kv unavailable")}

		err := publishJetstreamCompleteMsg(js, kv, payload, test.SilentLogger())

		assert.NotNil(t, err)
	})

	t.Run("success returns no error and writes kv", func(t *testing.T) {
		js := &test.MockJS{}
		kv := &test.MockKV{}

		err := publishJetstreamCompleteMsg(js, kv, payload, test.SilentLogger())

		assert.Nil(t, err)
		assert.Equal(t, "job-1.0", kv.PutKey)
	})
}
