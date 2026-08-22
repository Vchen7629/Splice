//go:build integration

package recombiner_test

import (
	"testing"

	"splice.com/go_services/internal/recombiner"
	shandler "splice.com/go_services/internal/shared/handler"
	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddI(t *testing.T) {
	t.Run("not ready until all chunks recorded, then returns the full chunk map", func(t *testing.T) {
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js, "recombine-chunk-recieved")

		ready, chunks, err := recombiner.Add(kv, shandler.ChunkCompleteMessage{
			JobID: "job-1", ChunkIndex: 0, TotalChunks: 2, StorageURL: "/tmp/chunk-0.mp4",
		}, test.SilentLogger())
		require.NoError(t, err)
		assert.False(t, ready)
		assert.Nil(t, chunks)

		ready, chunks, err = recombiner.Add(kv, shandler.ChunkCompleteMessage{
			JobID: "job-1", ChunkIndex: 1, TotalChunks: 2, StorageURL: "/tmp/chunk-1.mp4",
		}, test.SilentLogger())
		require.NoError(t, err)
		require.True(t, ready)
		require.Len(t, chunks, 2)
		assert.Equal(t, "/tmp/chunk-0.mp4", chunks[0])
		assert.Equal(t, "/tmp/chunk-1.mp4", chunks[1])
	})

	t.Run("multiple jobs are tracked independently", func(t *testing.T) {
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js, "recombine-chunk-recieved")

		_, _, err := recombiner.Add(kv, shandler.ChunkCompleteMessage{
			JobID: "job-A", ChunkIndex: 0, TotalChunks: 2, StorageURL: "/tmp/A-0.mp4",
		}, test.SilentLogger())
		require.NoError(t, err)

		readyB, chunksB, err := recombiner.Add(kv, shandler.ChunkCompleteMessage{
			JobID: "job-B", ChunkIndex: 0, TotalChunks: 1, StorageURL: "/tmp/B-0.mp4",
		}, test.SilentLogger())
		require.NoError(t, err)
		require.True(t, readyB)
		assert.Len(t, chunksB, 1, "job-B's chunk map must not include job-A's chunk")
	})
}
