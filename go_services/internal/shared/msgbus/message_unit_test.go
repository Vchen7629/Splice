//go:build unit

package msgbus_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/msgbus"
	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/test"
)

func TestAckWithErrHandling(t *testing.T) {
	t.Run("calls Ack on msg", func(t *testing.T) {
		msg := &test.MockMsg{}

		msgbus.AckWithErrHandling(test.SilentLogger(), msg)

		if !msg.AckCalled {
			t.Error("expected Ack to be called")
		}
	})

	t.Run("logs error when Ack fails", func(t *testing.T) {
		msg := &test.MockMsg{AckErr: errors.New("ack failed")}

		// Should not panic even when Ack returns an error
		msgbus.AckWithErrHandling(test.SilentLogger(), msg)
	})
}

func TestNakWithErrHandling(t *testing.T) {
	t.Run("calls Nak on msg", func(t *testing.T) {
		msg := &test.MockMsg{}

		msgbus.NakWithErrHandling(test.SilentLogger(), msg)

		if !msg.NakCalled {
			t.Error("expected Nak to be called")
		}
	})

	t.Run("logs error when Nak fails", func(t *testing.T) {
		msg := &test.MockMsg{NakErr: errors.New("nak failed")}

		// Should not panic even when Nak returns an error
		msgbus.NakWithErrHandling(test.SilentLogger(), msg)
	})
}

func TestDecode(t *testing.T) {
	t.Run("invalid JSON naks and does not ack", func(t *testing.T) {
		msg := &test.MockMsg{Payload: []byte("not valid json")}

		payload, ok := msgbus.Decode[pipeline.VideoJobMessage](msg, test.SilentLogger())

		require.False(t, ok)
		assert.NotNil(t, payload)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.AckCalled)
	})

	t.Run("invalid JSON with nak error logs and returns", func(t *testing.T) {
		nakErr := errors.New("nak failed")
		msg := &test.MockMsg{Payload: []byte("not valid json"), NakErr: nakErr}

		payload, ok := msgbus.Decode[pipeline.VideoJobMessage](msg, test.SilentLogger())

		require.False(t, ok)
		assert.NotNil(t, payload)
		assert.True(t, msg.NakCalled)
	})

}

func TestPublish(t *testing.T) {
	t.Run("publish error is returned", func(t *testing.T) {
		publishErr := errors.New("nats publish failed")
		mock := &test.MockJS{PublishErr: publishErr}

		err := msgbus.Publish(mock, pipeline.ChunkCompleteMessage{
			ChunkRef:    pipeline.ChunkRef{JobID: "job-1", ChunkIndex: 0},
			TotalChunks: 0,
			StorageURL:  "/output/chunk-0.mp4",
		}, "jobs.chunks.complete")

		assert.ErrorIs(t, err, publishErr)
	})
}
