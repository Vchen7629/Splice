//go:build unit

package jetstream_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/test"
)

func TestAckWithErrHandling(t *testing.T) {
	t.Run("calls Ack on msg", func(t *testing.T) {
		msg := &test.MockMsg{}

		sJetstream.AckWithErrHandling(test.SilentLogger(), msg)

		if !msg.AckCalled {
			t.Error("expected Ack to be called")
		}
	})

	t.Run("logs error when Ack fails", func(t *testing.T) {
		msg := &test.MockMsg{AckErr: errors.New("ack failed")}

		// Should not panic even when Ack returns an error
		sJetstream.AckWithErrHandling(test.SilentLogger(), msg)
	})
}

func TestNakWithErrHandling(t *testing.T) {
	t.Run("calls Nak on msg", func(t *testing.T) {
		msg := &test.MockMsg{}

		sJetstream.NakWithErrHandling(test.SilentLogger(), msg)

		if !msg.NakCalled {
			t.Error("expected Nak to be called")
		}
	})

	t.Run("logs error when Nak fails", func(t *testing.T) {
		msg := &test.MockMsg{NakErr: errors.New("nak failed")}

		// Should not panic even when Nak returns an error
		sJetstream.NakWithErrHandling(test.SilentLogger(), msg)
	})
}

func TestPublishChunkComplete(t *testing.T) {
	t.Run("publish error is returned", func(t *testing.T) {
		publishErr := errors.New("nats publish failed")
		mock := &test.MockJetStream{PublishErr: publishErr}

		err := sJetstream.PublishJetstreamMsg(mock, handler.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 0,
			StorageURL:  "/output/chunk-0.mp4",
		}, "jobs.chunks.complete")

		assert.ErrorIs(t, err, publishErr)
	})
}

func TestTerminateIfCancelled(t *testing.T) {
	t.Run("Naks and returns correct shouldCancel and stopJob when IsJobCancelled returns an err", func(t *testing.T) {
		mockKV := &test.MockKV{GetErr: errors.New("kv unavailable")}
		msg := &test.MockMsg{}

		shouldCancel, stopJob := sJetstream.TerminateIfCancelled(mockKV, msg, "job-1", test.SilentLogger())

		assert.False(t, shouldCancel)
		assert.True(t, stopJob)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.TermCalled)
	})

	t.Run("Returns false for both when job isnt cancelled", func(t *testing.T) {
		mockKV := &test.MockKV{}
		msg := &test.MockMsg{}

		shouldCancel, stopJob := sJetstream.TerminateIfCancelled(mockKV, msg, "job-1", test.SilentLogger())

		assert.False(t, shouldCancel)
		assert.False(t, stopJob)
		assert.False(t, msg.NakCalled)
		assert.False(t, msg.TermCalled)
	})

	t.Run("Returns true when msg termination fails", func(t *testing.T) {
		mockKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{"state":"CANCELLED","stage":"transcoder"}`),
		}
		msg := &test.MockMsg{TermErr: errors.New("term failed")}

		shouldCancel, stopJob := sJetstream.TerminateIfCancelled(mockKV, msg, "job-1", test.SilentLogger())

		assert.True(t, shouldCancel)
		assert.True(t, stopJob)
		assert.True(t, msg.TermCalled)
	})

	t.Run("Returns true when msg Term is successful", func(t *testing.T) {
		mockKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{"state":"CANCELLED","stage":"transcoder"}`),
		}
		msg := &test.MockMsg{}

		shouldCancel, stopJob := sJetstream.TerminateIfCancelled(mockKV, msg, "job-1", test.SilentLogger())

		assert.True(t, shouldCancel)
		assert.True(t, stopJob)
		assert.True(t, msg.TermCalled)
		assert.False(t, msg.NakCalled)
	})
}
