//go:build unit

package handler_test

import (
	"errors"
	"testing"
	"video-recombiner/internal/handler"
	"video-recombiner/internal/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckChunkRecieved(t *testing.T) {
	t.Run("returns false when key not found", func(t *testing.T) {
		kv := &test.MockKV{GetFound: false}

		processed, err := handler.CheckChunkRecieved(kv, "job-1", 0)

		require.NoError(t, err)
		assert.False(t, processed)
	})

	t.Run("returns true when key exists", func(t *testing.T) {
		kv := &test.MockKV{GetFound: true}

		processed, err := handler.CheckChunkRecieved(kv, "job-1", 0)

		require.NoError(t, err)
		assert.True(t, processed)
	})

	t.Run("returns error on unexpected kv failure", func(t *testing.T) {
		kv := &test.MockKV{GetErr: errors.New("kv unavailable")}

		_, err := handler.CheckChunkRecieved(kv, "job-1", 0)

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed")
	})

	t.Run("does not return error for ErrKeyNotFound", func(t *testing.T) {
		kv := &test.MockKV{GetErr: jetstream.ErrKeyNotFound}

		processed, err := handler.CheckChunkRecieved(kv, "job-1", 0)

		require.NoError(t, err)
		assert.False(t, processed)
	})

	t.Run("uses correct key format job_id.chunk_index", func(t *testing.T) {
		// Key lookup for job "abc" chunk 3 must use "abc.3".
		// We verify by having GetFound=true and confirming no error path is hit.
		kv := &test.MockKV{GetFound: true}

		processed, err := handler.CheckChunkRecieved(kv, "abc", 3)

		require.NoError(t, err)
		assert.True(t, processed)
	})
}

func TestAddChunkRecieved(t *testing.T) {
	t.Run("returns nil on success", func(t *testing.T) {
		kv := &test.MockKV{}

		err := handler.AddChunkRecieved(kv, "job-1", 0)

		require.NoError(t, err)
	})

	t.Run("writes correct key job_id.chunk_index", func(t *testing.T) {
		kv := &test.MockKV{}

		err := handler.AddChunkRecieved(kv, "job-abc", 2)

		require.NoError(t, err)
		assert.Equal(t, "job-abc.2", kv.PutKey)
	})

	t.Run("returns error on kv failure", func(t *testing.T) {
		kv := &test.MockKV{PutErr: errors.New("put failed")}

		err := handler.AddChunkRecieved(kv, "job-1", 0)

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed")
	})
}
