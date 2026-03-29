//go:build unit

package handler_test

import (
	"errors"
	"testing"
	"video-recombiner/internal/handler"
	"video-recombiner/internal/service"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
)

type mockJetStream struct {
	jetstream.JetStream
	publishErr error
}

func (m *mockJetStream) PublishAsync(_ string, _ []byte, _ ...jetstream.PublishOpt) (jetstream.PubAckFuture, error) {
	return nil, m.publishErr
}

func TestPublishVideoProcessingComplete(t *testing.T) {
	t.Run("returns wrapped error when publish fails", func(t *testing.T) {
		publishErr := errors.New("nats publish failed")
		mock := &mockJetStream{publishErr: publishErr}

		err := handler.PublishVideoProcessingComplete(mock, service.VideoProcessingCompleteMessage{
			JobID: "job-1",
		})

		assert.ErrorIs(t, err, publishErr)
	})
}
