//go:build unit

package handler_test

import (
	"context"
	"errors"
	"testing"
	"transcoder-worker/internal/handler"
	"transcoder-worker/internal/service"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
)

// mockJetStream embeds the interface and overrides only Publish
type mockJetStream struct {
	jetstream.JetStream
	publishErr error
}

func (m *mockJetStream) Publish(_ context.Context, _ string, _ []byte, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	return nil, m.publishErr
}

func TestPublishChunkComplete(t *testing.T) {
	t.Run("publish error is returned", func(t *testing.T) {
		publishErr := errors.New("nats publish failed")
		mock := &mockJetStream{publishErr: publishErr}

		fn := handler.PublishChunkComplete(mock)
		err := fn(service.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 0,
			StorageURL:  "/output/chunk-0.mp4",
		})

		assert.ErrorIs(t, err, publishErr)
	})
}
