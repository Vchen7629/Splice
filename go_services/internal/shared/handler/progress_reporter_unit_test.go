//go:build unit

package handler_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/handler"
	"splice.com/go_services/internal/shared/test"
)

type mockPublisher struct {
	calls  [][]byte
	failOn map[int]bool
}

func (m *mockPublisher) Publish(subj string, data []byte) error {
	m.calls = append(m.calls, data)
	if m.failOn[len(m.calls)] {
		return errors.New("publish failed")
	}
	return nil
}

func TestNewProgressReporter(t *testing.T) {
	t.Run("publishes progress with job id and stage", func(t *testing.T) {
		pub := &mockPublisher{}
		reporter := handler.NewProgressReporter(pub, "job-1", "transcode", test.SilentLogger())

		reporter(10)

		require.Len(t, pub.calls, 1)
		var msg handler.ProgressMessage
		require.NoError(t, json.Unmarshal(pub.calls[0], &msg))
		assert.Equal(t, handler.ProgressMessage{JobID: "job-1", Stage: "transcode", Progress: 10}, msg)
	})

	t.Run("skips repeated identical percentages", func(t *testing.T) {
		pub := &mockPublisher{}
		reporter := handler.NewProgressReporter(pub, "job-1", "transcode", test.SilentLogger())

		reporter(10)
		reporter(10)

		assert.Len(t, pub.calls, 1)
	})

	t.Run("publishes again once percentage changes", func(t *testing.T) {
		pub := &mockPublisher{}
		reporter := handler.NewProgressReporter(pub, "job-1", "transcode", test.SilentLogger())

		reporter(10)
		reporter(20)

		assert.Len(t, pub.calls, 2)
	})

	t.Run("retries percentage whose publish failed instead of treating it as sent", func(t *testing.T) {
		pub := &mockPublisher{failOn: map[int]bool{1: true}}
		reporter := handler.NewProgressReporter(pub, "job-1", "transcode", test.SilentLogger())

		reporter(30) // publish fails, no consumer recieves it
		reporter(30) // must retry rather than being skipped as duplicate

		assert.Len(t, pub.calls, 2)
	})
}
