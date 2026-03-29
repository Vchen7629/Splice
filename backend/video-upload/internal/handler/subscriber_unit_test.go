//go:build unit

package handler_test

import (
	"encoding/json"
	"errors"
	"testing"
	"video-upload/internal/handler"
	"video-upload/internal/service"
	"video-upload/internal/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMsg struct {
	jetstream.Msg
	data      []byte
	nakCalled bool
	ackCalled bool
	nakErr    error
	ackErr    error
}

func (m *mockMsg) Data() []byte { return m.data }
func (m *mockMsg) Nak() error   { m.nakCalled = true; return m.nakErr }
func (m *mockMsg) Ack() error   { m.ackCalled = true; return m.ackErr }

func TestReturnError(t *testing.T) {
	t.Run("invalid stream name should return error", func(t *testing.T) {
		lookupErr := errors.New("no stream")
		js := &test.MockJS{JStreamNameErr: lookupErr}

		_, _, err := handler.SubscribeJobCompletion(js, test.SilentLogger())

		require.Error(t, err)
		assert.ErrorIs(t, err, lookupErr)
	})

	t.Run("invalid stream should return error", func(t *testing.T) {
		streamErr := errors.New("stream err")
		js := &test.MockJS{JStreamErr: streamErr}

		_, _, err := handler.SubscribeJobCompletion(js, test.SilentLogger())

		require.Error(t, err)
		assert.ErrorIs(t, err, streamErr)
	})

	t.Run("consumererr should return error", func(t *testing.T) {
		consumerErr := errors.New("consumer error")
		js := &test.MockJS{JStream: &test.MockStream{ConsumerErr: consumerErr}}

		_, _, err := handler.SubscribeJobCompletion(js, test.SilentLogger())

		require.Error(t, err)
		assert.ErrorIs(t, err, consumerErr)
	})

	t.Run("failure to consume should return error", func(t *testing.T) {
		consumeErr := errors.New("consume error")
		js := &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{ConsumeErr: consumeErr}}}

		_, _, err := handler.SubscribeJobCompletion(js, test.SilentLogger())

		require.Error(t, err)
		assert.ErrorIs(t, err, consumeErr)
	})
}

func TestAckAndNacking(t *testing.T) {
	t.Run("invalid json should nak and not ack", func(t *testing.T) {
		msg := &mockMsg{data: []byte("not valid json")}
		consumer := &test.MockConsumer{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		_, consCtx, err := handler.SubscribeJobCompletion(js, test.SilentLogger())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
		assert.True(t, msg.nakCalled)
		assert.False(t, msg.ackCalled)
	})

	t.Run("valid json should ack and not nak", func(t *testing.T) {
		payload, err := json.Marshal(service.JobCompleteMessage{JobID: "job-1"})
		require.NoError(t, err)

		msg := &mockMsg{data: payload}
		consumer := &test.MockConsumer{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		_, _, err = handler.SubscribeJobCompletion(js, test.SilentLogger())

		require.NoError(t, err)
		assert.False(t, msg.nakCalled)
		assert.True(t, msg.ackCalled)
	})

	t.Run("nak error is handled without panic", func(t *testing.T) {
		msg := &mockMsg{data: []byte("not valid json"), nakErr: errors.New("nak failed")}
		consumer := &test.MockConsumer{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		assert.NotPanics(t, func() {
			handler.SubscribeJobCompletion(js, test.SilentLogger()) //nolint:errcheck
		})
		assert.True(t, msg.nakCalled)
	})

	t.Run("ack error is handled without panic", func(t *testing.T) {
		payload, err := json.Marshal(service.JobCompleteMessage{JobID: "job-1"})
		require.NoError(t, err)

		msg := &mockMsg{data: payload, ackErr: errors.New("ack failed")}
		consumer := &test.MockConsumer{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		assert.NotPanics(t, func() {
			handler.SubscribeJobCompletion(js, test.SilentLogger()) //nolint:errcheck
		})
		assert.True(t, msg.ackCalled)
	})
}

func TestTrackerPopulation(t *testing.T) {
	t.Run("valid message adds job ID to the returned tracker", func(t *testing.T) {
		payload, err := json.Marshal(service.JobCompleteMessage{JobID: "job-abc"})
		require.NoError(t, err)

		msg := &mockMsg{data: payload}
		consumer := &test.MockConsumer{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		tracker, _, err := handler.SubscribeJobCompletion(js, test.SilentLogger())

		require.NoError(t, err)
		assert.True(t, tracker.IsDone("job-abc"))
	})

	t.Run("invalid message does not add any job to the tracker", func(t *testing.T) {
		msg := &mockMsg{data: []byte("not valid json")}
		consumer := &test.MockConsumer{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		tracker, _, err := handler.SubscribeJobCompletion(js, test.SilentLogger())

		require.NoError(t, err)
		assert.False(t, tracker.IsDone("not valid json"))
	})

	t.Run("message with empty job_id is acked and adds empty string to tracker", func(t *testing.T) {
		payload, err := json.Marshal(service.JobCompleteMessage{JobID: ""})
		require.NoError(t, err)

		msg := &mockMsg{data: payload}
		consumer := &test.MockConsumer{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: consumer}}

		tracker, _, err := handler.SubscribeJobCompletion(js, test.SilentLogger())

		require.NoError(t, err)
		assert.True(t, msg.ackCalled)
		assert.True(t, tracker.IsDone(""))
	})

	t.Run("success returns non-nil tracker and consume context", func(t *testing.T) {
		js := &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{}}}

		tracker, consCtx, err := handler.SubscribeJobCompletion(js, test.SilentLogger())

		require.NoError(t, err)
		assert.NotNil(t, tracker)
		assert.NotNil(t, consCtx)
	})
}
