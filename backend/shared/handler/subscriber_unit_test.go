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
