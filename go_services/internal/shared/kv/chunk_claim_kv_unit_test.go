//go:build unit

package kv_test

import (
	"errors"
	"splice.com/go_services/internal/shared/kv"
	"splice.com/go_services/internal/shared/test"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimChunk(t *testing.T) {
	t.Run("returns true when claim is created", func(t *testing.T) {
		mockKV := &test.MockKV{}

		claimed, err := kv.ClaimChunk(mockKV, "job-1", 0)

		require.NoError(t, err)
		assert.True(t, claimed)
	})

	t.Run("returns false, no error when key already claimed", func(t *testing.T) {
		mockKV := &test.MockKV{CreateErr: jetstream.ErrKeyExists}

		claimed, err := kv.ClaimChunk(mockKV, "job-1", 0)

		require.NoError(t, err)
		assert.False(t, claimed)
	})

	t.Run("returns error on unexpected kv failure", func(t *testing.T) {
		mockKV := &test.MockKV{CreateErr: errors.New("kv unavailable")}

		_, err := kv.ClaimChunk(mockKV, "job-1", 0)

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed")
	})

	t.Run("uses correct key format job_id.chunk_index", func(t *testing.T) {
		mockKV := &test.MockKV{}

		_, err := kv.ClaimChunk(mockKV, "job-abc", 2)

		require.NoError(t, err)
		assert.Equal(t, "job-abc.2", mockKV.CreateKey)
	})
}

func TestReleaseChunkClaim(t *testing.T) {
	t.Run("returns nil on success", func(t *testing.T) {
		mockKV := &test.MockKV{}

		err := kv.ReleaseChunkClaim(mockKV, "job-1", 0)

		require.NoError(t, err)
	})

	t.Run("releases correct key job_id.chunk_index", func(t *testing.T) {
		mockKV := &test.MockKV{}

		err := kv.ReleaseChunkClaim(mockKV, "job-abc", 2)

		require.NoError(t, err)
		assert.Equal(t, "job-abc.2", mockKV.DeleteKey)
	})

	t.Run("returns error on kv failure", func(t *testing.T) {
		mockKV := &test.MockKV{DeleteErr: errors.New("delete failed")}

		err := kv.ReleaseChunkClaim(mockKV, "job-1", 0)

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed")
	})
}
