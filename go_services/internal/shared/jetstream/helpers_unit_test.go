//go:build unit

package jetstream_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/test"
)

func TestAckWithErrHandling(t *testing.T) {
	t.Run("calls Ack on msg", func(t *testing.T) {
		msg := &test.MockMsg{}

		sJetstream.AckWithErrHandling(test.SilentLogger(), msg)

		if !msg.AckCalled {
			t.Error("expected Ack to be called")
		}
	})

	t.Run("logs error when Ack fails", func(t *testing.T) {
		msg := &test.MockMsg{AckErr: errors.New("ack failed")}

		// Should not panic even when Ack returns an error
		sJetstream.AckWithErrHandling(test.SilentLogger(), msg)
	})
}

func TestNakWithErrHandling(t *testing.T) {
	t.Run("calls Nak on msg", func(t *testing.T) {
		msg := &test.MockMsg{}

		sJetstream.NakWithErrHandling(test.SilentLogger(), msg)

		if !msg.NakCalled {
			t.Error("expected Nak to be called")
		}
	})

	t.Run("logs error when Nak fails", func(t *testing.T) {
		msg := &test.MockMsg{NakErr: errors.New("nak failed")}

		// Should not panic even when Nak returns an error
		sJetstream.NakWithErrHandling(test.SilentLogger(), msg)
	})
}

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

		err := sJetstream.PublishJetstreamMsg(mock, handler.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 0,
			StorageURL:  "/output/chunk-0.mp4",
		}, "jobs.chunks.complete")

		assert.ErrorIs(t, err, publishErr)
	})
}
