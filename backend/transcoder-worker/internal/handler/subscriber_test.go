//go:build unit

package handler_test

import (
	"context"
	"errors"
	"testing"
	"transcoder-worker/internal/handler"
	"transcoder-worker/internal/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockJS struct {
	jetstream.JetStream
	streamNameErr error
	streamErr     error
	stream        jetstream.Stream
}

func (m *mockJS) StreamNameBySubject(_ context.Context, _ string) (string, error) {
	return "jobs", m.streamNameErr
}

func (m *mockJS) Stream(_ context.Context, _ string) (jetstream.Stream, error) {
	return m.stream, m.streamErr
}

type mockStream struct {
	jetstream.Stream
	consumerErr error
	consumer    jetstream.Consumer
}

func (m *mockStream) CreateOrUpdateConsumer(_ context.Context, _ jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	return m.consumer, m.consumerErr
}

type mockConsumer struct {
	jetstream.Consumer
	consumeErr   error
	msgToDeliver jetstream.Msg
}

func (m *mockConsumer) Consume(h jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	if m.consumeErr != nil {
		return nil, m.consumeErr
	}
	if m.msgToDeliver != nil {
		h(m.msgToDeliver)
	}
	return &mockConsumeCtx{}, nil
}

type mockConsumeCtx struct {
	jetstream.ConsumeContext
}

type mockMsg struct {
	jetstream.Msg
	data      []byte
	nakCalled bool
	ackCalled bool
}

func (m *mockMsg) Data() []byte { return m.data }
func (m *mockMsg) Nak() error   { m.nakCalled = true; return nil }
func (m *mockMsg) Ack() error   { m.ackCalled = true; return nil }

func TestStreamNameBySubjectFailsReturnsError(t *testing.T) {
	lookupErr := errors.New("no stream")
	js := &mockJS{streamNameErr: lookupErr}

	_, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, lookupErr)
}

func TestStreamFailsReturnsError(t *testing.T) {
	streamErr := errors.New("stream error")
	js := &mockJS{streamErr: streamErr}

	_, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, streamErr)
}

func TestCreateConsumerFailsReturnsError(t *testing.T) {
	consumerErr := errors.New("consumer error")
	js := &mockJS{stream: &mockStream{consumerErr: consumerErr}}

	_, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, consumerErr)
}

func TestConsumeFailsReturnsError(t *testing.T) {
	consumeErr := errors.New("consume error")
	js := &mockJS{stream: &mockStream{consumer: &mockConsumer{consumeErr: consumeErr}}}

	_, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, consumeErr)
}

func TestInvalidJSONNaksAndDoesNotAck(t *testing.T) {
	msg := &mockMsg{data: []byte("not valid json")}
	js := &mockJS{stream: &mockStream{consumer: &mockConsumer{msgToDeliver: msg}}}

	consCtx, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

	require.NoError(t, err)
	assert.NotNil(t, consCtx)
	assert.True(t, msg.nakCalled)
	assert.False(t, msg.ackCalled)
}
