//go:build unit

package handler

import (
	"encoding/json"
	"errors"
	"testing"
	"video-status/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenJobComplete_NakErrors(t *testing.T) {
	nakErr := errors.New("nak failed")

	tests := []struct {
		name   string
		msg    *test.MockMsg
		mockKV *test.MockKV
	}{
		{
			name:   "nak error after unmarshal failure does not panic",
			msg:    &test.MockMsg{Payload: []byte("not valid json{{"), NakErr: nakErr},
			mockKV: test.NewMockKV(),
		},
		{
			name: "nak error after kv Put failure does not panic",
			msg: &test.MockMsg{Payload: func() []byte {
				b, _ := json.Marshal(map[string]string{"job_id": "job-nak-kv"})
				return b
			}(), NakErr: nakErr},
			mockKV: func() *test.MockKV {
				kv := test.NewMockKV()
				kv.PutErr = errors.New("kv unavailable")
				return kv
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cons := &test.MockConsumer{Msg: tc.msg}
			js := &test.MockJS{JStream: &test.MockStream{Cons: cons}}

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
		cons := &test.MockConsumer{Msg: msg}
		js := &test.MockJS{JStream: &test.MockStream{Cons: cons}}

		_, err := ListenJobComplete(js, test.NewMockKV(), test.SilentLogger())

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
		js      *test.MockJS
		wantErr error
	}{
		{
			name:    "stream name lookup failure returns error",
			js:      &test.MockJS{JStreamNameErr: streamNameErr},
			wantErr: streamNameErr,
		},
		{
			name:    "stream fetch failure returns error",
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
			_, err := ListenJobComplete(tc.js, test.NewMockKV(), test.SilentLogger())

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}
