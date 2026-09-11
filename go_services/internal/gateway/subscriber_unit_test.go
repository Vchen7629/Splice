//go:build unit

package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenJobCompleteU(t *testing.T) {
	nakErr := errors.New("nak failed")

	nakErrTests := []struct {
		name   string
		msg    *test.MockMsg
		mockKV *MockKV
	}{
		{
			name:   "nak error after unmarshal failure does not panic",
			msg:    &test.MockMsg{Payload: []byte("not valid json{{"), NakErr: nakErr},
			mockKV: NewMockKV(),
		},
		{
			name: "nak error after kv Update failure does not panic",
			msg: &test.MockMsg{Payload: func() []byte {
				b, _ := json.Marshal(map[string]string{"job_id": "job-nak-kv"})
				return b
			}(), NakErr: nakErr},
			mockKV: func() *MockKV {
				kv := NewMockKV()
				kv.UpdateErr = errors.New("kv unavailable")
				return kv
			}(),
		},
	}

	for _, tc := range nakErrTests {
		t.Run(tc.name, func(t *testing.T) {
			cons := &test.MockConsumerWithMsg{Msg: tc.msg}
			js := &MockJS{JStream: &test.MockStream{Cons: cons}}

			_, err := ListenJobComplete(js, tc.mockKV, test.SilentLogger())

			require.NoError(t, err)
			assert.True(t, tc.msg.NakCalled, "expected Nak to be called")
		})
	}

	t.Run("ack error after successful kv Put does not panic", func(t *testing.T) {
		b, _ := json.Marshal(map[string]string{"job_id": "job-ack-err"})
		msg := &test.MockMsg{Payload: b, AckErr: errors.New("ack failed")}
		cons := &test.MockConsumerWithMsg{Msg: msg}
		js := &MockJS{JStream: &test.MockStream{Cons: cons}}

		_, err := ListenJobComplete(js, NewMockKV(), test.SilentLogger())

		require.NoError(t, err)
		assert.True(t, msg.AckCalled, "expected Ack to be called")
	})

	streamNameErr := errors.New("no stream for subject")
	streamErr := errors.New("stream fetch failed")
	consumerErr := errors.New("create consumer failed")
	consumeErr := errors.New("consume failed")

	returnErrorTests := []struct {
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
			js:      &MockJS{JStream: &test.MockStream{ConsumerErr: consumerErr}},
			wantErr: consumerErr,
		},
		{
			name:    "consume failure returns error",
			js:      &MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{ConsumeErr: consumeErr}}},
			wantErr: consumeErr,
		},
	}

	for _, tc := range returnErrorTests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ListenJobComplete(tc.js, NewMockKV(), test.SilentLogger())

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}

	t.Run("KV Update conflict naks the message for redelivery instead of overwriting", func(t *testing.T) {
		kv := NewMockKV()
		kv.UpdateErr = jetstream.ErrKeyExists
		msg := &test.MockMsg{Payload: mustMarshalJobStatic("job-conflict")}
		cons := &test.MockConsumerWithMsg{Msg: msg}
		js := &MockJS{JStream: &test.MockStream{Cons: cons}}

		_, err := ListenJobComplete(js, kv, test.SilentLogger())

		require.NoError(t, err)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.AckCalled)
	})
}

func TestIsJobTerminal(t *testing.T) {
	t.Run("key that doesnt exist just returns false and revision 0", func(t *testing.T) {
		mockKV := &test.MockKV{}

		revision, isTerminal, err := isJobTerminal(mockKV, "some-jobID")

		require.NoError(t, err)
		assert.False(t, isTerminal)
		assert.Zero(t, revision)
	})

	t.Run("error fetching from keyValue returns revision 0, false, and error", func(t *testing.T) {
		mockKV := &test.MockKV{GetErr: errors.New("Some error")}

		revision, isTerminal, err := isJobTerminal(mockKV, "some-jobID")

		require.Error(t, err)
		assert.Equal(t, err.Error(), "failed to fetch from kv: Some error")
		assert.False(t, isTerminal)
		assert.Zero(t, revision)
	})

	t.Run("returns false and revision 0 if the its invalid json", func(t *testing.T) {
		mockKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{[`),
		}

		revision, isTerminal, err := isJobTerminal(mockKV, "job-1")

		require.Error(t, err)
		assert.Equal(t, err.Error(), "failed to unmarshal json: invalid character '[' looking for beginning of object key string")
		assert.False(t, isTerminal)
		assert.Zero(t, revision)
	})

	tests := []struct {
		state string
	}{
		{state: "COMPLETE"},
		{state: "FAILED"},
		{state: "CANCELLED"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("returns true if the KV state is in terminal state (%s)", tc.state), func(t *testing.T) {
			b, err := json.Marshal(sJetstream.MilestoneStatus{State: tc.state, Stage: "transcoder"})
			require.NoError(t, err)

			mockKV := &test.MockKV{
				GetFound:         true,
				GetValue:         b,
				GetEntryRevision: 5,
			}

			revision, isTerminal, err := isJobTerminal(mockKV, "job-1")

			require.NoError(t, err)
			assert.True(t, isTerminal)
			assert.NotZero(t, revision)
		})
	}

	t.Run("returns false if the KV state is non terminal state", func(t *testing.T) {
		mockKV := &test.MockKV{
			GetFound:         true,
			GetValue:         []byte(`{"state":"PROCESSING","stage":"transcoder"}`),
			GetEntryRevision: 5,
		}

		revision, isTerminal, err := isJobTerminal(mockKV, "job-1")

		require.NoError(t, err)
		assert.False(t, isTerminal)
		assert.NotZero(t, revision)
	})
}
