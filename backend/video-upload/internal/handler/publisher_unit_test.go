//go:build unit

package handler_test

import (
	"errors"
	"testing"
	"video-upload/internal/handler"
	"video-upload/internal/service"
	"video-upload/internal/test"

	"github.com/stretchr/testify/assert"
)

func TestCatchesError(t *testing.T) {
	t.Run("nats publish errors", func(t *testing.T) {
		publishErr := errors.New("nats publish failed")
		mock := &test.MockJS{PublishErr: publishErr}

		err := handler.PublishVideoMetadata(mock, service.SceneSplitMessage{
			JobID: "job-1",
		})

		assert.ErrorIs(t, err, publishErr)
	})
}
