//go:build unit

package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenJobComplete_NakErrors(t *testing.T) {
	nakErr := errors.New("nak failed")

	tests := []struct {
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
			name: "nak error after kv Put failure does not panic",
			msg: &test.MockMsg{Payload: func() []byte {
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
			cons := &test.MockConsumerWithMsg{Msg: tc.msg}
			js := &MockJS{JStream: &test.MockStream{Cons: cons}}

			_, err := ListenJobComplete(js, tc.mockKV, test.SilentLogger())

			require.NoError(t, err)
			assert.True(t, tc.msg.NakCalled, "expected Nak to be called")
		})
	}
}

func TestListenJobComplete_AckError(t *testing.T) {
	t.Run("ack error after successful kv Put does not panic", func(t *testing.T) {
		b, _ := json.Marshal(map[string]string{"job_id": "job-ack-err"})
		msg := &test.MockMsg{Payload: b, AckErr: errors.New("ack failed")}
		cons := &test.MockConsumerWithMsg{Msg: msg}
		js := &MockJS{JStream: &test.MockStream{Cons: cons}}

		_, err := ListenJobComplete(js, NewMockKV(), test.SilentLogger())

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
			js:      &MockJS{JStream: &test.MockStream{ConsumerErr: consumerErr}},
			wantErr: consumerErr,
		},
		{
			name:    "consume failure returns error",
			js:      &MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{ConsumeErr: consumeErr}}},
			wantErr: consumeErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ListenJobComplete(tc.js, NewMockKV(), test.SilentLogger())

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestIsJobTerminal(t *testing.T) {
	t.Run("key that doesnt exist just returns false", func(t *testing.T) {
		mockKV := &test.MockKV{}

		isTerminal, err := isJobTerminal(mockKV, "some-jobID")

		require.NoError(t, err)
		assert.False(t, isTerminal)
	})

	t.Run("error fetching from keyValue returns false and error", func(t *testing.T) {
		mockKV := &test.MockKV{GetErr: errors.New("Some error")}

		isTerminal, err := isJobTerminal(mockKV, "some-jobID")

		require.Error(t, err)
		assert.Equal(t, err.Error(), "failed to fetch from kv: Some error")
		assert.False(t, isTerminal)
	})

	t.Run("returns false if the its invalid json", func(t *testing.T) {
		mockKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{[`),
		}

		isTerminal, err := isJobTerminal(mockKV, "job-1")

		require.Error(t, err)
		assert.Equal(t, err.Error(), "failed to unmarshal json: invalid character '[' looking for beginning of object key string")
		assert.False(t, isTerminal)
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
			b, err := json.Marshal(jetstream.MilestoneStatus{State: tc.state, Stage: "transcoder"})
			require.NoError(t, err)

			mockKV := &test.MockKV{
				GetFound: true,
				GetValue: b,
			}

			isTerminal, err := isJobTerminal(mockKV, "job-1")

			require.NoError(t, err)
			assert.True(t, isTerminal)
		})
	}

	t.Run("returns false if the KV state is non terminal state", func(t *testing.T) {
		mockKV := &test.MockKV{
			GetFound: true,
			GetValue: []byte(`{"state":"PROCESSING","stage":"transcoder"}`),
		}

		isTerminal, err := isJobTerminal(mockKV, "job-1")

		require.NoError(t, err)
		assert.False(t, isTerminal)
	})
}
