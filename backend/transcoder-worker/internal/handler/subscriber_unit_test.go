//go:build unit

package handler_test

import (
	"encoding/json"
	"errors"
	"testing"
	"transcoder-worker/internal/handler"
	"transcoder-worker/internal/service"
	"transcoder-worker/internal/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMsg stubs jetstream.Msg for message-handling tests.
// It is kept here rather than in internal/test because it is only
// needed for subscriber behaviour and carries no value elsewhere.
type mockMsg struct {
	jetstream.Msg
	data      []byte
	nakCalled bool
	ackCalled bool
}

func (m *mockMsg) Data() []byte { return m.data }
func (m *mockMsg) Nak() error   { m.nakCalled = true; return nil }
func (m *mockMsg) Ack() error   { m.ackCalled = true; return nil }

func TestReturnError(t *testing.T) {
	t.Run("stream name lookup failure returns error", func(t *testing.T) {
		lookupErr := errors.New("no stream")
		js := &test.MockJS{JStreamNameErr: lookupErr}

		_, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

		require.Error(t, err)
		assert.ErrorIs(t, err, lookupErr)
	})

	t.Run("stream failure returns error", func(t *testing.T) {
		streamErr := errors.New("stream error")
		js := &test.MockJS{JStreamErr: streamErr}

		_, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

		require.Error(t, err)
		assert.ErrorIs(t, err, streamErr)
	})

	t.Run("create consumer failure returns error", func(t *testing.T) {
		consumerErr := errors.New("consumer error")
		js := &test.MockJS{JStream: &test.MockStream{ConsumerErr: consumerErr}}

		_, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

		require.Error(t, err)
		assert.ErrorIs(t, err, consumerErr)
	})

	t.Run("consume failure returns error", func(t *testing.T) {
		consumeErr := errors.New("consume error")
		js := &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{ConsumeErr: consumeErr}}}

		_, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

		require.Error(t, err)
		assert.ErrorIs(t, err, consumeErr)
	})
}

func TestAckAndNacking(t *testing.T) {
	t.Run("invalid JSON naks and does not ack", func(t *testing.T) {
		msg := &mockMsg{data: []byte("not valid json")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		consCtx, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.nakCalled)
		assert.False(t, msg.ackCalled)
	})

	t.Run("valid JSON naks when transcode fails", func(t *testing.T) {
		payload, err := json.Marshal(service.VideoChunkMessage{
			JobID:            "job-1",
			ChunkIndex:       0,
			StoragePath:      "nonexistent",
			TargetResolution: "720p",
		})
		require.NoError(t, err)

		msg := &mockMsg{data: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		// TranscodeVideo will fail (nonexistent file) so we expect a Nak, not Ack
		_, err = handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.True(t, msg.nakCalled)
		assert.False(t, msg.ackCalled)
	})
}
