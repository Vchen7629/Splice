//go:build unit

package handler_test

import (
	"encoding/json"
	"errors"
	"testing"
	"video-recombiner/internal/handler"
	"video-recombiner/internal/service"
	"video-recombiner/internal/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMsg struct {
	jetstream.Msg
	data      []byte
	ackErr    error
	nakCalled bool
	ackCalled bool
}

func (m *mockMsg) Data() []byte { return m.data }
func (m *mockMsg) Nak() error   { m.nakCalled = true; return nil }
func (m *mockMsg) Ack() error   { m.ackCalled = true; return m.ackErr }

func TestReturnError(t *testing.T) {
	streamNameErr := errors.New("no stream")
	streamErr := errors.New("stream error")
	consumerErr := errors.New("consumer error")
	consumeErr := errors.New("consume error")

	tests := []struct {
		name    string
		js      *test.MockJS
		wantErr error
	}{
		{
			name:    "stream name lookup failure returns error",
			js:      &test.MockJS{JStreamNameErr: streamNameErr},
			wantErr: streamNameErr,
		},
		{
			name:    "stream failure returns error",
			js:      &test.MockJS{JStreamErr: streamErr},
			wantErr: streamErr,
		},
		{
			name:    "create consumer failure returns error",
			js:      &test.MockJS{JStream: &test.MockStream{ConsumerErr: consumerErr}},
			wantErr: consumerErr,
		},
		{
			name:    "consume failure returns error",
			js:      &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{ConsumeErr: consumeErr}}},
			wantErr: consumeErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handler.RecombineVideo(tc.js, test.SilentLogger(), "http://storage")

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestMessageHandling(t *testing.T) {
	t.Run("invalid JSON naks and does not ack", func(t *testing.T) {
		msg := &mockMsg{data: []byte("not valid json")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		consCtx, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.nakCalled)
		assert.False(t, msg.ackCalled)
	})

	t.Run("partial chunk acks without combining", func(t *testing.T) {
		// Only the first of two chunks arrives — tracker not yet ready.
		payload, err := json.Marshal(service.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 2,
			StorageURL:  "http://storage/chunk-0.mp4",
		})
		require.NoError(t, err)

		msg := &mockMsg{data: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		consCtx, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.ackCalled)
		assert.False(t, msg.nakCalled)
	})

	t.Run("all chunks ready acks and triggers combine even if download fails", func(t *testing.T) {
		// TotalChunks=1, ChunkIndex=0 — immediately ready.
		// HTTP download will fail on invalid URL, but msg must be acked before that.
		payload, err := json.Marshal(service.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 1,
			StorageURL:  "http://127.0.0.1:0/nonexistent-chunk.mp4",
		})
		require.NoError(t, err)

		msg := &mockMsg{data: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		consCtx, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.ackCalled)
		assert.False(t, msg.nakCalled)
	})

	t.Run("ack failure does not trigger combine", func(t *testing.T) {
		// When Ack returns an error the handler returns early before downloading chunks.
		payload, err := json.Marshal(service.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 1,
			StorageURL:  "http://storage/chunk-0.mp4",
		})
		require.NoError(t, err)

		msg := &mockMsg{data: payload, ackErr: errors.New("ack failed")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		consCtx, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.ackCalled)
		assert.False(t, msg.nakCalled)
	})
}
