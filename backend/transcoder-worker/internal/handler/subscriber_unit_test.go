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
			_, err := handler.ConsumeVideoChunk("http://storage", tc.js, test.SilentLogger())

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestAckAndNacking(t *testing.T) {
	t.Run("invalid JSON naks and does not ack", func(t *testing.T) {
		msg := &mockMsg{data: []byte("not valid json")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		consCtx, err := handler.ConsumeVideoChunk("http://storage", js, test.SilentLogger())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.nakCalled)
		assert.False(t, msg.ackCalled)
	})

	t.Run("fetch failure does not nak or ack", func(t *testing.T) {
		payload, err := json.Marshal(service.VideoChunkMessage{
			JobID:            "job-1",
			ChunkIndex:       0,
			StorageURL:       "http://localhost:1/job-1/chunk.mp4",
			TargetResolution: "720p",
		})
		require.NoError(t, err)

		msg := &mockMsg{data: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		_, err = handler.ConsumeVideoChunk("http://storage", js, test.SilentLogger())

		require.NoError(t, err)
		assert.False(t, msg.nakCalled)
		assert.False(t, msg.ackCalled)
	})
}
