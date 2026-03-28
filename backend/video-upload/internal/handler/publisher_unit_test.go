//go:build unit

package handler_test

import (
	"context"
	"errors"
	"testing"
	"video-upload/internal/handler"
	"video-upload/internal/service"

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

func TestCatchesError(t *testing.T) {
	t.Run("nats publish errors", func(t *testing.T) {
		publishErr := errors.New("nats publish failed")
		mock := &mockJetStream{publishErr: publishErr}

		err := handler.PublishVideoMetadata(mock, service.SceneSplitMessage{
			JobID: "job-1",
		})

		assert.ErrorIs(t, err, publishErr)
	})
}
