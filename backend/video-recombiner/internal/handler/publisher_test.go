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

// mockJetStream embeds the interface and overrides only Publish
type mockJetStream struct {
	jetstream.JetStream
	publishErr error
}

func (m *mockJetStream) PublishAsync(_ string, _ []byte, _ ...jetstream.PublishOpt) (jetstream.PubAckFuture, error) {
	return nil, m.publishErr
}

func TestPublishError(t *testing.T) {
	publishErr := errors.New("nats publish failed")
	mock := &mockJetStream{publishErr: publishErr}

	err := handler.PublishVideoProcessingComplete(mock, service.VideoProcessingCompleteMessage{
		JobID: "job-1",
	})

	assert.ErrorIs(t, err, publishErr)
}
