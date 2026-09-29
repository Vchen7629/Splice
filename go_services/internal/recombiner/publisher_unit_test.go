//go:build unit

package recombiner

import (
	"errors"
	"testing"

	"splice.com/go_services/internal/shared/handler"
	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
)

func TestPublishJetstreamCompleteMsg(t *testing.T) {
	payload := handler.ChunkCompleteMessage{ChunkRef: handler.ChunkRef{JobID: "job-1", ChunkIndex: 0}}

	t.Run("publish failure returns the err", func(t *testing.T) {
		js := &test.MockJS{PublishErr: errors.New("nats unavailable")}
		kv := &test.MockKV{}

		err := publishJetstreamCompleteMsg(js, kv, payload, test.SilentLogger())

		assert.NotNil(t, err)
	})

	t.Run("kv write failure returns err after publish succeeds", func(t *testing.T) {
		js := &test.MockJS{}
		kv := &test.MockKV{PutErr: errors.New("kv unavailable")}

		err := publishJetstreamCompleteMsg(js, kv, payload, test.SilentLogger())

		assert.NotNil(t, err)
	})

	t.Run("success returns no error and writes kv", func(t *testing.T) {
		js := &test.MockJS{}
		kv := &test.MockKV{}

		err := publishJetstreamCompleteMsg(js, kv, payload, test.SilentLogger())

		assert.Nil(t, err)
		assert.Equal(t, "job-1.0", kv.PutKey)
	})
}
