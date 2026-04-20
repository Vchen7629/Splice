//go:build unit

package handler_test

import (
	"errors"
	"shared/handler"
	"shared/test"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReturnError(t *testing.T) {
	streamNameErr := errors.New("no stream")
	streamErr := errors.New("stream error")
	consumerErr := errors.New("consumer error")

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
			name:    "stream failure returns error",
			js:      &test.MockJS{JStreamErr: streamErr},
			wantErr: streamErr,
		},
		{
			name:    "create consumer failure returns error",
			js:      &test.MockJS{JStream: &test.MockStream{ConsumerErr: consumerErr}},
			wantErr: consumerErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handler.CreateDurableConsumer(tc.js, "idk", "idk")

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestUnmarshalJetstreamMsg(t *testing.T) {
	t.Run("invalid JSON naks and does not ack", func(t *testing.T) {
		msg := &test.MockMsg{Payload: []byte("not valid json")}

		payload, ok := handler.UnmarshalJetstreamMsg[handler.VideoJobMessage](msg, test.SilentLogger())

		require.False(t, ok)
		assert.NotNil(t, payload)
		assert.True(t, msg.NakCalled)
		assert.False(t, msg.AckCalled)
	})

	t.Run("invalid JSON with nak error logs and returns", func(t *testing.T) {
		nakErr := errors.New("nak failed")
		msg := &test.MockMsg{Payload: []byte("not valid json"), NakErr: nakErr}

		payload, ok := handler.UnmarshalJetstreamMsg[handler.VideoJobMessage](msg, test.SilentLogger())

		require.False(t, ok)
		assert.NotNil(t, payload)
		assert.True(t, msg.NakCalled)
	})

}
