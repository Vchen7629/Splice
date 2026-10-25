//go:build unit

package converter

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/test"
)

func TestPublishJetstreamProcessedMsg(t *testing.T) {
	t.Run("publish failure returns err and does not mark job processed", func(t *testing.T) {
		js := &test.MockJS{PublishErr: errors.New("nats unavailable")}
		kv := &test.MockKV{}

		err := publishJetstreamProcessedMsg(js, kv, "job-1", test.SilentLogger())

		require.Error(t, err)
		assert.Empty(t, kv.PutKey)
	})

	t.Run("success marks job processed under the job id", func(t *testing.T) {
		kv := &test.MockKV{}

		err := publishJetstreamProcessedMsg(&test.MockJS{}, kv, "job-1", test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, "job-1", kv.PutKey)
	})
}
