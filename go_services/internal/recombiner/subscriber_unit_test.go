//go:build unit

package recombiner_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"splice.com/go_services/internal/recombiner"
	shandler "splice.com/go_services/internal/shared/handler"
	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ackWaitU = 30 * time.Second

func validPayload(t *testing.T, jobID string) []byte {
	t.Helper()
	data, err := json.Marshal(shandler.ChunkCompleteMessage{
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
			_, err := recombiner.RecombineVideo(tc.js, nil, &test.MockKV{}, &test.MockKV{}, &test.MockKV{}, ackWaitU, test.SilentLogger(), "http://storage")

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

		consCtx, err := recombiner.RecombineVideo(js, nil, &test.MockKV{}, &test.MockKV{}, &test.MockKV{}, ackWaitU, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.AckCalled)
	})

	t.Run("ack failure on a non-triggering chunk still persists kv", func(t *testing.T) {
		// AddChunkKV runs before Ack for a non-triggering chunk, so it succeeds even if Ack later fails.
		payload, err := json.Marshal(shandler.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 2, // not ready — combine never runs
			StorageURL:  "http://storage/chunk-0.mp4",
		})
		require.NoError(t, err)

		msg := &test.MockMsg{Payload: payload, AckErr: errors.New("ack failed")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{}

		consCtx, err := recombiner.RecombineVideo(js, nil, kv, &test.MockKV{}, &test.MockKV{}, ackWaitU, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
		assert.Equal(t, "job-1.0", kv.PutKey)
	})

	t.Run("non-triggering chunk does not advance job milestone", func(t *testing.T) {
		// since transcode workers are horizontally scaled/multiple workers processing multiple chunks concurrently
		// chunk-complete msgs arrive while most chunks are still encoding, milestone must only advance once
		// combining actually begines not on every chunk recieved
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		jobMilestoneKV := &test.MockKV{}

		_, err := recombiner.RecombineVideo(js, nil, &test.MockKV{}, jobMilestoneKV, &test.MockKV{}, ackWaitU, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.Empty(t, jobMilestoneKV.CreateKey, "milestone should not be written until triggering chunk arrives")
	})

	t.Run("triggering chunk advances the job milestone", func(t *testing.T) {
		payload, err := json.Marshal(shandler.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 1,
			StorageURL:  "http://storage/chunk-0.mp4",
		})
		require.NoError(t, err)

		msg := &test.MockMsg{Payload: payload}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		jobMilestoneKV := &test.MockKV{}

		_, err = recombiner.RecombineVideo(js, nil, &test.MockKV{}, jobMilestoneKV, &test.MockKV{}, ackWaitU, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.Equal(t, "job-1", jobMilestoneKV.CreateKey)
	})
}

func TestIdempotency(t *testing.T) {
	t.Run("already processed chunk acks and skips processing", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetFound: true}

		_, err := recombiner.RecombineVideo(js, nil, kv, &test.MockKV{}, &test.MockKV{}, ackWaitU, test.SilentLogger(), "http://storage")

		require.NoError(t, err)
		assert.True(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
	})

	t.Run("already processed chunk does not write to kv again", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetFound: true}

		_, err := recombiner.RecombineVideo(js, nil, kv, &test.MockKV{}, &test.MockKV{}, ackWaitU, test.SilentLogger(), "http://storage")

		require.NoError(t, err)
		assert.Empty(t, kv.PutKey)
	})

	t.Run("kv check error does not ack or nak", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		kv := &test.MockKV{GetErr: errors.New("kv unavailable")}

		_, err := recombiner.RecombineVideo(js, nil, kv, &test.MockKV{}, &test.MockKV{}, ackWaitU, test.SilentLogger(), "http://storage")

		require.NoError(t, err)
		assert.False(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
	})

	t.Run("kv key format is job_id.chunk_index", func(t *testing.T) {
		jobID := "abc-123"
		chunkIndex := 3
		expected := fmt.Sprintf("%s.%d", jobID, chunkIndex)
		assert.Equal(t, "abc-123.3", expected)
	})
}

func TestCancelledCases(t *testing.T) {
	t.Run("cancelled job terminates message and does no other work", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		processedKV := &test.MockKV{}
		jobMilestoneKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{"state":"CANCELLED","stage":"recombiner"}`),
		}
		claimKV := &test.MockKV{}

		_, err := recombiner.RecombineVideo(js, nil, processedKV, jobMilestoneKV, claimKV, 15*time.Second, test.SilentLogger(), "http://storage")

		require.NoError(t, err)
		assert.True(t, msg.TermCalled)
		assert.False(t, msg.AckCalled)
		assert.False(t, msg.NakCalled)
		assert.Empty(t, processedKV.PutKey, "cancelled gate should bail before dedupe/combine work")
		assert.Empty(t, claimKV.CreateKey, "cancelled gate should never claim the chunk")
	})

	t.Run("failing to terminate a msg on cancelled job is logged not naked", func(t *testing.T) {
		msg := &test.MockMsg{Payload: validPayload(t, "job-1"), TermErr: errors.New("term failed")}
		consumer := &test.MockConsumerWithMsg{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}
		jobMilestoneKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{"state":"CANCELLED","stage":"recombiner"}`),
		}

		_, err := recombiner.RecombineVideo(js, nil, &test.MockKV{}, jobMilestoneKV, &test.MockKV{}, 15*time.Second, test.SilentLogger(), "http://storage")

		require.NoError(t, err)
		assert.True(t, msg.TermCalled)
		assert.False(t, msg.NakCalled)
	})
}
