//go:build unit

package msgbus

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/test"
)

func TestIsProcesseds(t *testing.T) {
	t.Run("returns false when key not found", func(t *testing.T) {
		mockProcessedKV := &test.MockKV{GetFound: false}

		processed, err := IsProcessed(mockProcessedKV, "job-1.0")

		require.NoError(t, err)
		assert.False(t, processed)
	})

	t.Run("returns true when key exists", func(t *testing.T) {
		mockProcessedKV := &test.MockKV{GetFound: true}

		processed, err := IsProcessed(mockProcessedKV, "job-1.0")

		require.NoError(t, err)
		assert.True(t, processed)
	})

	t.Run("returns error on unexpected kv failure", func(t *testing.T) {
		mockProcessedKV := &test.MockKV{GetErr: errors.New("kv unavailable")}

		_, err := IsProcessed(mockProcessedKV, "job-1.0")

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed")
	})
}

func TestMarkProcessed(t *testing.T) {
	t.Run("writes under the given key", func(t *testing.T) {
		mockKV := &test.MockKV{}

		err := MarkProcessed(mockKV, "job-abc.2", test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, "job-abc.2", mockKV.PutKey)
	})

	t.Run("returns error on kv failure", func(t *testing.T) {
		mockKV := &test.MockKV{PutErr: errors.New("put failed")}

		err := MarkProcessed(mockKV, "job-abc.2", test.SilentLogger())

		require.Error(t, err)
	})
}

func TestClaimChunk(t *testing.T) {
	t.Run("returns true when claim is created", func(t *testing.T) {
		mockKV := &test.MockKV{}

		claimed, err := claimChunk(mockKV, "job-1", 0)

		require.NoError(t, err)
		assert.True(t, claimed)
	})

	t.Run("returns false, no error when key already claimed", func(t *testing.T) {
		mockKV := &test.MockKV{CreateErr: jetstream.ErrKeyExists}

		claimed, err := claimChunk(mockKV, "job-1", 0)

		require.NoError(t, err)
		assert.False(t, claimed)
	})

	t.Run("returns error on unexpected kv failure", func(t *testing.T) {
		mockKV := &test.MockKV{CreateErr: errors.New("kv unavailable")}

		_, err := claimChunk(mockKV, "job-1", 0)

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed")
	})

	t.Run("uses correct key format job_id.chunk_index", func(t *testing.T) {
		mockKV := &test.MockKV{}

		_, err := claimChunk(mockKV, "job-abc", 2)

		require.NoError(t, err)
		assert.Equal(t, "job-abc.2", mockKV.CreateKey)
	})
}

func TestReleaseChunkClaim(t *testing.T) {
	t.Run("returns nil on success", func(t *testing.T) {
		mockKV := &test.MockKV{}

		err := releaseChunkClaim(mockKV, "job-1", 0)

		require.NoError(t, err)
	})

	t.Run("releases correct key job_id.chunk_index", func(t *testing.T) {
		mockKV := &test.MockKV{}

		err := releaseChunkClaim(mockKV, "job-abc", 2)

		require.NoError(t, err)
		assert.Equal(t, "job-abc.2", mockKV.DeleteKey)
	})

	t.Run("returns error on kv failure", func(t *testing.T) {
		mockKV := &test.MockKV{DeleteErr: errors.New("delete failed")}

		err := releaseChunkClaim(mockKV, "job-1", 0)

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed")
	})
}
