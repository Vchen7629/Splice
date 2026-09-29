//go:build unit

package transcoder

import (
	"errors"
	"splice.com/go_services/internal/shared/handler"
	"testing"

	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
)

// mockPublisher stubs handler.Publisher for progress-reporting assertions.
type mockPublisher struct {
	published []string
	err       error
}

func (m *mockPublisher) Publish(subj string, _ []byte) error {
	m.published = append(m.published, subj)
	return m.err
}

func TestPublishJetstreamProcessedMsg(t *testing.T) {
	payload := VideoChunkMessage{ChunkRef: handler.ChunkRef{JobID: "job-abc", ChunkIndex: 2}, TotalChunks: 4}

	t.Run("publish failure does not write kv or ack and err", func(t *testing.T) {
		js := &test.MockJS{PublishErr: errors.New("nats unavailable")}
		kv := &test.MockKV{}
		pub := &mockPublisher{}

		err := publishJetstreamProcessedMsg(pub, js, kv, payload, "http://storage/chunk.mp4", test.SilentLogger())

		assert.NotNil(t, err)
		assert.Empty(t, kv.PutKey)
	})

	t.Run("kv write failure returns err after publish succeeds but kv put fails", func(t *testing.T) {
		js := &test.MockJS{}
		kv := &test.MockKV{PutErr: errors.New("kv unavailable")}
		pub := &mockPublisher{}

		err := publishJetstreamProcessedMsg(pub, js, kv, payload, "http://storage/chunk.mp4", test.SilentLogger())

		assert.NotNil(t, err)
	})

	t.Run("success returns no error, writes kv, and reports progress", func(t *testing.T) {
		js := &test.MockJS{}
		kv := &test.MockKV{}
		pub := &mockPublisher{}

		err := publishJetstreamProcessedMsg(pub, js, kv, payload, "http://storage/chunk.mp4", test.SilentLogger())

		assert.Nil(t, err)
		assert.Equal(t, "job-abc.2", kv.PutKey)
		assert.Contains(t, pub.published, "progress.job-abc")
	})
}
