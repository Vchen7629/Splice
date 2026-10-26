//go:build unit

package msgbus

import (
	"errors"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumeDurable(t *testing.T) {
	t.Run("Consume fail returns error", func(t *testing.T) {
		consumeErr := errors.New("consume error")
		js := &test.MockJS{JStream: &test.MockStream{Cons: &test.MockConsumer{ConsumeErr: consumeErr}}}

		_, err := ConsumeDurable(js, ConsumerConfig{
			Subject:       "idk",
			ServiceName:   "idk",
			AckWait:       30 * time.Second,
			MaxAckPending: 10,
		}, nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, consumeErr)
	})
}

func TestCreateDurableConsumer(t *testing.T) {
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
			_, err := createDurableConsumer(tc.js, "idk", "idk", 30*time.Second, 10)

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}
