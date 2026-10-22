//go:build unit

package jetstream_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestUnmarshalJetstreamMsg(t *testing.T) {
	t.Run("invalid JSON naks and does not ack", func(t *testing.T) {
		msg := &test.MockMsg{Payload: []byte("not valid json")}

		payload, ok := sJetstream.UnmarshalJetstreamMsg[handler.VideoJobMessage](msg, test.SilentLogger())

		require.False(t, ok)
		assert.NotNil(t, payload)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.AckCalled)
	})

	t.Run("invalid JSON with nak error logs and returns", func(t *testing.T) {
		nakErr := errors.New("nak failed")
		msg := &test.MockMsg{Payload: []byte("not valid json"), NakErr: nakErr}

		payload, ok := sJetstream.UnmarshalJetstreamMsg[handler.VideoJobMessage](msg, test.SilentLogger())

		require.False(t, ok)
		assert.NotNil(t, payload)
		assert.True(t, msg.NakCalled)
	})

}

func TestPublishChunkComplete(t *testing.T) {
	t.Run("publish error is returned", func(t *testing.T) {
		publishErr := errors.New("nats publish failed")
		mock := &test.MockJS{PublishErr: publishErr}

		err := sJetstream.PublishJetstreamMsg(mock, handler.ChunkCompleteMessage{
			ChunkRef:    handler.ChunkRef{JobID: "job-1", ChunkIndex: 0},
			TotalChunks: 0,
			StorageURL:  "/output/chunk-0.mp4",
		}, "jobs.chunks.complete")

		assert.ErrorIs(t, err, publishErr)
	})
}

func TestTerminateIfCancelled(t *testing.T) {
	t.Run("Naks and returns correct shouldCancel and stopJob when GetMilestoneKV returns an err", func(t *testing.T) {
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
			GetValue: []byte(`{"state":"CANCELLED","stage":"chunk-worker"}`),
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
			GetValue: []byte(`{"state":"CANCELLED","stage":"chunk-worker"}`),
		}
		msg := &test.MockMsg{}

		shouldCancel, stopJob := sJetstream.TerminateIfCancelled(mockKV, msg, "job-1", test.SilentLogger())

		assert.True(t, shouldCancel)
		assert.True(t, stopJob)
		assert.True(t, msg.TermCalled)
		assert.False(t, msg.NakCalled)
	})
}
