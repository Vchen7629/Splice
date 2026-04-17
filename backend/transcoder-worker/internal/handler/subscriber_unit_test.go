//go:build unit

package handler_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"transcoder-worker/internal/handler"
	"transcoder-worker/internal/service"
	"transcoder-worker/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validPayload(t *testing.T, jobID string) []byte {
	t.Helper()
	data, err := json.Marshal(service.VideoChunkMessage{
		JobID:            jobID,
		ChunkIndex:       0,
		StorageURL:       "http://localhost:1/job-1/chunk.mp4",
		TargetResolution: "720p",
	})
	require.NoError(t, err)
	return data
}

func TestConsumeFailReturnError(t *testing.T) {
	consumeErr := errors.New("consume error")

	js := &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{ConsumeErr: consumeErr}}}

	_, err := handler.ConsumeVideoChunk("http://storage", js, &test.MockKV{}, &test.MockKV{}, test.SilentLogger())

	require.Error(t, err)
	assert.ErrorIs(t, err, consumeErr)
}

func TestFetchFailureNaks(t *testing.T) {
	msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
	consumer := &test.MockConsumerWithMsg{Msg: msg}
	js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

	_, err := handler.ConsumeVideoChunk("http://storage", js, &test.MockKV{}, &test.MockKV{}, test.SilentLogger())

	require.NoError(t, err)
	assert.True(t, msg.NakCalled)
}

func TestIdempotency(t *testing.T) {
	t.Run("already processed chunk acks and skips processing", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetFound: true}

		_, err := handler.ConsumeVideoChunk("http://storage", js, kv, &test.MockKV{}, test.SilentLogger())

		require.NoError(t, err)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
	})

	t.Run("already processed chunk does not write to kv again", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetFound: true}

		_, err := handler.ConsumeVideoChunk("http://storage", js, kv, &test.MockKV{}, test.SilentLogger())

		require.NoError(t, err)
		assert.Empty(t, kv.PutKey)
	})

	t.Run("kv check error does not ack or nak", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetErr: errors.New("kv unavailable")}

		_, err := handler.ConsumeVideoChunk("http://storage", js, kv, &test.MockKV{}, test.SilentLogger())

		require.NoError(t, err)
		assert.False(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
	})

	t.Run("writes kv with correct key on success", func(t *testing.T) {
		payload, err := json.Marshal(service.VideoChunkMessage{
			JobID:            "job-abc",
			ChunkIndex:       2,
			StorageURL:       "http://localhost:1/job-abc/chunk.mp4",
			TargetResolution: "480p",
		})
		require.NoError(t, err)

		msg := &test.MockMsg{Payload: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{}

		_, _ = handler.ConsumeVideoChunk("http://localhost:1", js, kv, &test.MockKV{}, test.SilentLogger())

		assert.Empty(t, kv.PutKey, "kv.Put should not be called when processing fails")
	})

	t.Run("kv key format is job_id.chunk_index", func(t *testing.T) {
		jobID := "abc-123"
		chunkIndex := 3
		expected := fmt.Sprintf("%s.%d", jobID, chunkIndex)
		assert.Equal(t, "abc-123.3", expected)
	})
}
