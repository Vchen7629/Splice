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

func TestRecombineVideoErrors(t *testing.T) {
	t.Run("invalid stream name returns error", func(t *testing.T) {
		lookupErr := errors.New("no stream")
		js := &test.MockJS{JStreamNameErr: lookupErr}

		_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

		require.Error(t, err)
		assert.ErrorIs(t, err, lookupErr)
	})

	t.Run("invalid stream returns error", func(t *testing.T) {
		streamErr := errors.New("stream error")
		js := &test.MockJS{JStreamErr: streamErr}

		_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

		require.Error(t, err)
		assert.ErrorIs(t, err, streamErr)
	})

	t.Run("consumer creation failure returns error", func(t *testing.T) {
		consumerErr := errors.New("consumer error")
		js := &test.MockJS{JStream: &test.MockStream{ConsumerErr: consumerErr}}

		_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

		require.Error(t, err)
		assert.ErrorIs(t, err, consumerErr)
	})

	t.Run("consume failure returns error", func(t *testing.T) {
		consumeErr := errors.New("consume error")
		js := &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{ConsumeErr: consumeErr}}}

		_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

		require.Error(t, err)
		assert.ErrorIs(t, err, consumeErr)
	})
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
	})

	t.Run("all chunks ready acks and triggers combine even if ffmpeg fails", func(t *testing.T) {
		// TotalChunks=1, ChunkIndex=0 — immediately ready.
		// ffmpeg will fail on the nonexistent path, but msg must be acked before that.
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
	})

	t.Run("ack failure does not trigger combine", func(t *testing.T) {
		// When Ack returns an error the handler returns early.
		// Verified by checking no manifest file was written.
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
	})
}
