//go:build unit

package handler_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"video-recombiner/internal/handler"
	"video-recombiner/internal/service"
	"video-recombiner/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validPayload(t *testing.T, jobID string) []byte {
	t.Helper()
	data, err := json.Marshal(service.ChunkCompleteMessage{
		JobID:       jobID,
		ChunkIndex:  0,
		TotalChunks: 2, // not ready — combine never runs
		StorageURL:  "http://localhost:1/job-1/chunk.mp4",
	})
	require.NoError(t, err)
	return data
}

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
			_, err := handler.RecombineVideo(tc.js, &test.MockKV{}, &test.MockKV{}, test.SilentLogger(), "http://storage")

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestMessageHandling(t *testing.T) {
	t.Run("invalid JSON naks and does not ack", func(t *testing.T) {
		msg := &test.MockMsg{Payload: []byte("not valid json")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		consCtx, err := handler.RecombineVideo(js, &test.MockKV{}, &test.MockKV{}, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.AckCalled)
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

		msg := &test.MockMsg{Payload: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		consCtx, err := handler.RecombineVideo(js, &test.MockKV{}, &test.MockKV{}, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
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

		msg := &test.MockMsg{Payload: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		consCtx, err := handler.RecombineVideo(js, &test.MockKV{}, &test.MockKV{}, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
	})

	t.Run("ack failure does not trigger combine or write kv", func(t *testing.T) {
		// When Ack returns an error the handler returns early before downloading chunks.
		payload, err := json.Marshal(service.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 1,
			StorageURL:  "http://storage/chunk-0.mp4",
		})
		require.NoError(t, err)

		msg := &test.MockMsg{Payload: payload, AckErr: errors.New("ack failed")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{}

		consCtx, err := handler.RecombineVideo(js, kv, &test.MockKV{}, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
		assert.Empty(t, kv.PutKey)
	})
}

func TestIdempotency(t *testing.T) {
	t.Run("already processed chunk acks and skips processing", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetFound: true}

		_, err := handler.RecombineVideo(js, kv, &test.MockKV{}, test.SilentLogger(), "http://storage")

		require.NoError(t, err)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
	})

	t.Run("already processed chunk does not write to kv again", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetFound: true}

		_, err := handler.RecombineVideo(js, kv, &test.MockKV{}, test.SilentLogger(), "http://storage")

		require.NoError(t, err)
		assert.Empty(t, kv.PutKey)
	})

	t.Run("kv check error does not ack or nak", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetErr: errors.New("kv unavailable")}

		_, err := handler.RecombineVideo(js, kv, &test.MockKV{}, test.SilentLogger(), "http://storage")

		require.NoError(t, err)
		assert.False(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
	})

	t.Run("writes kv with correct key after ack", func(t *testing.T) {
		payload, err := json.Marshal(service.ChunkCompleteMessage{
			JobID:       "job-abc",
			ChunkIndex:  2,
			TotalChunks: 3, // not ready — combine never runs, but KV write still happens
			StorageURL:  "http://localhost:1/job-abc/chunk.mp4",
		})
		require.NoError(t, err)

		msg := &test.MockMsg{Payload: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{}

		_, err = handler.RecombineVideo(js, kv, &test.MockKV{}, test.SilentLogger(), "http://storage")

		require.NoError(t, err)
		assert.Equal(t, "job-abc.2", kv.PutKey)
	})

	t.Run("kv key format is job_id.chunk_index", func(t *testing.T) {
		jobID := "abc-123"
		chunkIndex := 3
		expected := fmt.Sprintf("%s.%d", jobID, chunkIndex)
		assert.Equal(t, "abc-123.3", expected)
	})
}
