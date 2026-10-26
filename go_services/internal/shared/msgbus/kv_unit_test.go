//go:build unit

package msgbus

import (
	"errors"
	"testing"

	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPutKeyKV(t *testing.T) {
	t.Run("writes correct key job_id.chunk_index", func(t *testing.T) {
		mockKV := &test.MockKV{}

		err := PutKeyKV(mockKV, "job-abc.2", []byte("processed"))

		require.NoError(t, err)
		assert.Equal(t, "job-abc.2", mockKV.PutKey)
	})

	t.Run("returns error on kv failure", func(t *testing.T) {
		mockKV := &test.MockKV{PutErr: errors.New("put failed")}

		err := PutKeyKV(mockKV, "job-1", []byte("processed"))

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed")
	})
}
