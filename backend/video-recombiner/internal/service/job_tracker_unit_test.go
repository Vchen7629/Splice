//go:build unit

package service

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdd(t *testing.T) {
	t.Run("creates state for new job", func(t *testing.T) {
		tracker := NewJobTracker()

		ready, chunks := tracker.Add("job-1", 0, "/tmp/chunk-0.mp4", 3)

		assert.False(t, ready)
		assert.Nil(t, chunks)
		tracker.mu.Lock()
		state, ok := tracker.jobs["job-1"]
		tracker.mu.Unlock()
		require.True(t, ok)
		assert.Equal(t, 3, state.totalChunks)
		assert.Equal(t, "/tmp/chunk-0.mp4", state.chunks[0])
	})

	t.Run("returns false and nil when not all chunks received", func(t *testing.T) {
		tracker := NewJobTracker()

		ready, chunks := tracker.Add("job-1", 0, "/tmp/chunk-0.mp4", 3)
		assert.False(t, ready)
		assert.Nil(t, chunks)

		ready, chunks = tracker.Add("job-1", 1, "/tmp/chunk-1.mp4", 3)
		assert.False(t, ready)
		assert.Nil(t, chunks)
	})

	t.Run("returns true and all chunks when complete", func(t *testing.T) {
		tracker := NewJobTracker()

		tracker.Add("job-1", 0, "/tmp/chunk-0.mp4", 3)
		tracker.Add("job-1", 1, "/tmp/chunk-1.mp4", 3)
		ready, chunks := tracker.Add("job-1", 2, "/tmp/chunk-2.mp4", 3)

		assert.True(t, ready)
		require.NotNil(t, chunks)
		assert.Equal(t, "/tmp/chunk-0.mp4", chunks[0])
		assert.Equal(t, "/tmp/chunk-1.mp4", chunks[1])
		assert.Equal(t, "/tmp/chunk-2.mp4", chunks[2])
	})

	t.Run("deletes job state after completion", func(t *testing.T) {
		tracker := NewJobTracker()

		tracker.Add("job-1", 0, "/tmp/chunk-0.mp4", 1)

		tracker.mu.Lock()
		_, exists := tracker.jobs["job-1"]
		tracker.mu.Unlock()
		assert.False(t, exists)
	})

	t.Run("single chunk job completes immediately", func(t *testing.T) {
		tracker := NewJobTracker()

		ready, chunks := tracker.Add("job-single", 0, "/tmp/only-chunk.mp4", 1)

		assert.True(t, ready)
		require.NotNil(t, chunks)
		assert.Equal(t, "/tmp/only-chunk.mp4", chunks[0])
	})

	t.Run("completed job can be re-added as a new job", func(t *testing.T) {
		tracker := NewJobTracker()

		ready, _ := tracker.Add("job-1", 0, "/tmp/chunk-0.mp4", 1)
		require.True(t, ready)

		ready, chunks := tracker.Add("job-1", 0, "/tmp/new-chunk-0.mp4", 2)
		assert.False(t, ready)
		assert.Nil(t, chunks)
		tracker.mu.Lock()
		state, ok := tracker.jobs["job-1"]
		tracker.mu.Unlock()
		require.True(t, ok)
		assert.Equal(t, 2, state.totalChunks)
	})

	t.Run("duplicate chunk index overwrites previous path", func(t *testing.T) {
		tracker := NewJobTracker()

		tracker.Add("job-1", 0, "/tmp/first.mp4", 2)
		tracker.Add("job-1", 0, "/tmp/overwritten.mp4", 2)
		ready, chunks := tracker.Add("job-1", 0, "/tmp/overwritten-again.mp4", 2)

		assert.False(t, ready)
		assert.Nil(t, chunks)
		tracker.mu.Lock()
		state := tracker.jobs["job-1"]
		tracker.mu.Unlock()
		assert.Equal(t, "/tmp/overwritten-again.mp4", state.chunks[0])
		assert.Len(t, state.chunks, 1)
	})

	t.Run("multiple jobs are tracked independently", func(t *testing.T) {
		tracker := NewJobTracker()

		tracker.Add("job-A", 0, "/tmp/A-0.mp4", 2)
		tracker.Add("job-B", 0, "/tmp/B-0.mp4", 1)

		readyB, chunksB := tracker.Add("job-B", 0, "/tmp/B-0.mp4", 1)
		assert.True(t, readyB)
		assert.NotNil(t, chunksB)

		tracker.mu.Lock()
		_, existsA := tracker.jobs["job-A"]
		tracker.mu.Unlock()
		assert.True(t, existsA)
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Run("concurrent adds to the same job do not race", func(t *testing.T) {
		tracker := NewJobTracker()
		const numGoroutines = 50
		const totalChunks = numGoroutines

		var wg sync.WaitGroup
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				tracker.Add("shared-job", idx, "/tmp/chunk.mp4", totalChunks)
			}(i)
		}
		wg.Wait()

		tracker.mu.Lock()
		_, exists := tracker.jobs["shared-job"]
		tracker.mu.Unlock()
		assert.False(t, exists, "job should be complete and deleted after all chunks received")
	})

	t.Run("concurrent adds to different jobs do not race", func(t *testing.T) {
		tracker := NewJobTracker()
		const numJobs = 20

		var wg sync.WaitGroup
		results := make([]bool, numJobs)

		for i := 0; i < numJobs; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				jobID := "job-" + string(rune('A'+idx))
				ready, _ := tracker.Add(jobID, 0, "/tmp/chunk.mp4", 1)
				results[idx] = ready
			}(i)
		}
		wg.Wait()

		for i, ready := range results {
			assert.True(t, ready, "job %d should have completed", i)
		}
	})
}
