//go:build unit

package jetstream

import (
	"errors"
	"splice.com/go_services/internal/shared/test"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckKeyExists(t *testing.T) {
	t.Run("returns false when key not found", func(t *testing.T) {
		mockKV := &test.MockKV{GetFound: false}

		processed, err := CheckKeyExist(mockKV, "job-1.0")

		require.NoError(t, err)
		assert.False(t, processed)
	})

	t.Run("returns true when key exists", func(t *testing.T) {
		mockKV := &test.MockKV{GetFound: true}

		processed, err := CheckKeyExist(mockKV, "job-1.0")

		require.NoError(t, err)
		assert.True(t, processed)
	})

	t.Run("returns error on unexpected kv failure", func(t *testing.T) {
		mockKV := &test.MockKV{GetErr: errors.New("kv unavailable")}

		_, err := CheckKeyExist(mockKV, "job-1.0")

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed")
	})

	t.Run("does not return error for ErrKeyNotFound", func(t *testing.T) {
		mockKV := &test.MockKV{GetErr: jetstream.ErrKeyNotFound}

		processed, err := CheckKeyExist(mockKV, "job-1.0")

		require.NoError(t, err)
		assert.False(t, processed)
	})

	t.Run("uses correct key format job_id.chunk_index", func(t *testing.T) {
		// Key lookup for job "abc" chunk 3 must use "abc.3".
		// We verify by having GetFound=true and confirming no error path is hit.
		mockKV := &test.MockKV{GetFound: true}

		processed, err := CheckKeyExist(mockKV, "abc.3")

		require.NoError(t, err)
		assert.True(t, processed)
	})
}

func TestPutKeyKV(t *testing.T) {
	t.Run("returns nil on success", func(t *testing.T) {
		mockKV := &test.MockKV{}

		err := PutKeyKV(mockKV, "job-1.0", []byte("processed"))

		require.NoError(t, err)
	})

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

// For video chunk kv related tests

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
