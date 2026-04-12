//go:build unit

package handler

import (
	"errors"
	"testing"
	"video-status/internal/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
