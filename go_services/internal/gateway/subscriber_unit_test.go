//go:build unit

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	stest "splice.com/go_services/internal/shared/test"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenJobComplete_NakErrors(t *testing.T) {
	nakErr := errors.New("nak failed")

	tests := []struct {
		name   string
		msg    *MockMsg
		mockKV *MockKV
	}{
		{
			name:   "nak error after unmarshal failure does not panic",
			msg:    &MockMsg{Payload: []byte("not valid json{{"), NakErr: nakErr},
			mockKV: NewMockKV(),
		},
		{
			name: "nak error after kv Put failure does not panic",
			msg: &MockMsg{Payload: func() []byte {
				b, _ := json.Marshal(map[string]string{"job_id": "job-nak-kv"})
				return b
			}(), NakErr: nakErr},
			mockKV: func() *MockKV {
				kv := NewMockKV()
				kv.PutErr = errors.New("kv unavailable")
				return kv
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cons := &MockConsumer{Msg: tc.msg}
			js := &MockJS{JStream: &MockStream{Cons: cons}}

			_, err := ListenJobComplete(js, tc.mockKV, stest.SilentLogger())

			require.NoError(t, err)
			assert.True(t, tc.msg.NakCalled, "expected Nak to be called")
		})
	}
}

func TestListenJobComplete_AckError(t *testing.T) {
	t.Run("ack error after successful kv Put does not panic", func(t *testing.T) {
		b, _ := json.Marshal(map[string]string{"job_id": "job-ack-err"})
		msg := &MockMsg{Payload: b, AckErr: errors.New("ack failed")}
		cons := &MockConsumer{Msg: msg}
		js := &MockJS{JStream: &MockStream{Cons: cons}}

		_, err := ListenJobComplete(js, NewMockKV(), stest.SilentLogger())

		require.NoError(t, err)
		assert.True(t, msg.AckCalled, "expected Ack to be called")
	})
}

func TestListenJobComplete_ReturnErrors(t *testing.T) {
	streamNameErr := errors.New("no stream for subject")
	streamErr := errors.New("stream fetch failed")
	consumerErr := errors.New("create consumer failed")
	consumeErr := errors.New("consume failed")

	tests := []struct {
		name    string
		js      *MockJS
		wantErr error
	}{
		{
			name:    "stream name lookup failure returns error",
			js:      &MockJS{JStreamNameErr: streamNameErr},
			wantErr: streamNameErr,
		},
		{
			name:    "stream fetch failure returns error",
			js:      &MockJS{JStreamErr: streamErr},
			wantErr: streamErr,
		},
		{
			name:    "create consumer failure returns error",
			js:      &MockJS{JStream: &MockStream{ConsumerErr: consumerErr}},
			wantErr: consumerErr,
		},
		{
			name:    "consume failure returns error",
			js:      &MockJS{JStream: &MockStream{Cons: &MockConsumer{ConsumeErr: consumeErr}}},
			wantErr: consumeErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ListenJobComplete(tc.js, NewMockKV(), stest.SilentLogger())

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// MockStream stubs jetstream.Stream.
// The consumer field is named Cons to avoid conflicting with the Consumer()
// method promoted by the embedded interface.
type MockStream struct {
	jetstream.Stream
	ConsumerErr error
	Cons        jetstream.Consumer
}

func (m *MockStream) CreateOrUpdateConsumer(_ context.Context, _ jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	return m.Cons, m.ConsumerErr
}

// MockConsumer stubs jetstream.Consumer.
// If Msg is set it is delivered to the handler when Consume is called.
type MockConsumer struct {
	jetstream.Consumer
	ConsumeErr error
	Msg        jetstream.Msg
}

func (m *MockConsumer) Consume(h jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	if m.ConsumeErr != nil {
		return nil, m.ConsumeErr
	}
	if m.Msg != nil {
		h(m.Msg)
	}
	return &MockConsumeCtx{}, nil
}

// MockConsumeCtx stubs jetstream.ConsumeContext.
type MockConsumeCtx struct {
	jetstream.ConsumeContext
	Stopped bool
}

func (m *MockConsumeCtx) Stop() { m.Stopped = true }

// MockMsg stubs jetstream.Msg for unit-testing the Consume handler.
type MockMsg struct {
	jetstream.Msg
	Payload   []byte
	AckErr    error
	NakErr    error
	AckCalled bool
	NakCalled bool
}

func (m *MockMsg) Data() []byte { return m.Payload }

func (m *MockMsg) Ack() error {
	m.AckCalled = true
	return m.AckErr
}

func (m *MockMsg) Nak() error {
	m.NakCalled = true
	return m.NakErr
}
