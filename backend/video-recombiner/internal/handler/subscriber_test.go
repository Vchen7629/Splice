//go:build unit

package handler_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"video-recombiner/internal/handler"
	"video-recombiner/internal/service"
	"video-recombiner/internal/test"

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
	ackErr    error
	nakCalled bool
	ackCalled bool
}

func (m *mockMsg) Data() []byte { return m.data }
func (m *mockMsg) Nak() error   { m.nakCalled = true; return nil }
func (m *mockMsg) Ack() error   { m.ackCalled = true; return m.ackErr }

func TestStreamNameBySubjectFailsReturnsError(t *testing.T) {
	lookupErr := errors.New("no stream")
	js := &test.MockJS{JStreamNameErr: lookupErr}

	_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, lookupErr)
}

func TestStreamFailsReturnsError(t *testing.T) {
	streamErr := errors.New("stream error")
	js := &test.MockJS{JStreamErr: streamErr}

	_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, streamErr)
}

func TestCreateConsumerFailsReturnsError(t *testing.T) {
	consumerErr := errors.New("consumer error")
	js := &test.MockJS{JStream: &test.MockStream{ConsumerErr: consumerErr}}

	_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, consumerErr)
}

func TestConsumeFailsReturnsError(t *testing.T) {
	consumeErr := errors.New("consume error")
	js := &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{ConsumeErr: consumeErr}}}

	_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, consumeErr)
}

func TestInvalidJSONNaksAndDoesNotAck(t *testing.T) {
	msg := &mockMsg{data: []byte("not valid json")}
	consumer := &test.MockConsumerWithMsg{Msg: msg}
	js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

	consCtx, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

	require.NoError(t, err)
	assert.NotNil(t, consCtx)
	assert.True(t, msg.nakCalled)
	assert.False(t, msg.ackCalled)
}

// Only the first of two chunks arrives — tracker not yet ready, so no combine triggered.
func TestPartialChunkAcksWithoutCombining(t *testing.T) {
	payload, err := json.Marshal(service.ChunkCompleteMessage{
		JobID:       "job-1",
		ChunkIndex:  0,
		TotalChunks: 2,
		OutputPath:  "chunk-0.mp4",
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
}

// TotalChunks=1, ChunkIndex=0 — tracker is immediately ready and CombineChunks is called.
// ffmpeg will fail on the nonexistent input path, but msg must have been acked before that.
func TestAllChunksReadyCombineFailsStillAcked(t *testing.T) {
	payload, err := json.Marshal(service.ChunkCompleteMessage{
		JobID:       "job-1",
		ChunkIndex:  0,
		TotalChunks: 1,
		OutputPath:  "nonexistent-chunk.mp4",
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
}

// When Ack returns an error the handler returns early — CombineChunks must not be called.
// We verify this by checking that no manifest file was written to the output dir.
func TestAckFailsDoesNotCombine(t *testing.T) {
	payload, err := json.Marshal(service.ChunkCompleteMessage{
		JobID:       "job-1",
		ChunkIndex:  0,
		TotalChunks: 1,
		OutputPath:  "chunk-0.mp4",
	})
	require.NoError(t, err)

	outputDir := t.TempDir()
	msg := &mockMsg{data: payload, ackErr: errors.New("ack failed")}
	consumer := &test.MockConsumerWithMsg{Msg: msg}
	js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

	consCtx, err := handler.RecombineVideo(js, test.SilentLogger(), outputDir)

	require.NoError(t, err)
	assert.NotNil(t, consCtx)
	assert.True(t, msg.ackCalled)
	assert.False(t, msg.nakCalled)

	manifest := filepath.Join(outputDir, "jobs", "job-1", "manifest.txt")
	_, statErr := os.Stat(manifest)
	assert.True(t, os.IsNotExist(statErr), "manifest should not exist when ack fails before combine")
}
