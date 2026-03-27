//go:build unit

package service

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateNewJobTracker(t *testing.T) {
	tracker := NewJobTracker()
	require.NotNil(t, tracker)
	assert.NotNil(t, tracker.jobs)
	assert.Empty(t, tracker.jobs)
}

func TestCreatesStateForNewJob(t *testing.T) {
	tracker := NewJobTracker()

	ready, chunks := tracker.Add("job-1", 0, "/tmp/chunk-0.mp4", 3)

	assert.False(t, ready)
	assert.Nil(t, chunks)
	// state should exist internally
	tracker.mu.Lock()
	state, ok := tracker.jobs["job-1"]
	tracker.mu.Unlock()
	require.True(t, ok)
	assert.Equal(t, 3, state.totalChunks)
	assert.Equal(t, "/tmp/chunk-0.mp4", state.chunks[0])
}

func TestReturnsFalseNilWhenNotAllChunksReceived(t *testing.T) {
	tracker := NewJobTracker()

	ready, chunks := tracker.Add("job-1", 0, "/tmp/chunk-0.mp4", 3)
	assert.False(t, ready)
	assert.Nil(t, chunks)

	ready, chunks = tracker.Add("job-1", 1, "/tmp/chunk-1.mp4", 3)
	assert.False(t, ready)
	assert.Nil(t, chunks)
}

func TestReturnsTrueAndChunksWhenAllChunksReceived(t *testing.T) {
	tracker := NewJobTracker()

	tracker.Add("job-1", 0, "/tmp/chunk-0.mp4", 3)
	tracker.Add("job-1", 1, "/tmp/chunk-1.mp4", 3)
	ready, chunks := tracker.Add("job-1", 2, "/tmp/chunk-2.mp4", 3)

	assert.True(t, ready)
	require.NotNil(t, chunks)
	assert.Equal(t, "/tmp/chunk-0.mp4", chunks[0])
	assert.Equal(t, "/tmp/chunk-1.mp4", chunks[1])
	assert.Equal(t, "/tmp/chunk-2.mp4", chunks[2])
}

func TestDeletesJobStateAfterCompletion(t *testing.T) {
	tracker := NewJobTracker()

	tracker.Add("job-1", 0, "/tmp/chunk-0.mp4", 1)

	tracker.mu.Lock()
	_, exists := tracker.jobs["job-1"]
	tracker.mu.Unlock()

	assert.False(t, exists, "job state should be deleted after all chunks received")
}

func TestSingleChunkJobCompletesImmediately(t *testing.T) {
	tracker := NewJobTracker()

	ready, chunks := tracker.Add("job-single", 0, "/tmp/only-chunk.mp4", 1)

	assert.True(t, ready)
	require.NotNil(t, chunks)
	assert.Equal(t, "/tmp/only-chunk.mp4", chunks[0])
}

func TestCompletedJobCanBeReAddedAsNewJob(t *testing.T) {
	tracker := NewJobTracker()

	// complete the job
	ready, _ := tracker.Add("job-1", 0, "/tmp/chunk-0.mp4", 1)
	require.True(t, ready)

	// same jobID treated as a fresh job
	ready, chunks := tracker.Add("job-1", 0, "/tmp/new-chunk-0.mp4", 2)
	assert.False(t, ready)
	assert.Nil(t, chunks)

	tracker.mu.Lock()
	state, ok := tracker.jobs["job-1"]
	tracker.mu.Unlock()
	require.True(t, ok)
	assert.Equal(t, 2, state.totalChunks)
}

func TestDuplicateChunkIndexOverwrites(t *testing.T) {
	tracker := NewJobTracker()

	tracker.Add("job-1", 0, "/tmp/first.mp4", 2)
	// overwrite chunk 0 with a different path
	tracker.Add("job-1", 0, "/tmp/overwritten.mp4", 2)

	// only 1 unique index, so not ready yet
	ready, chunks := tracker.Add("job-1", 0, "/tmp/overwritten-again.mp4", 2)
	assert.False(t, ready)
	assert.Nil(t, chunks)

	tracker.mu.Lock()
	state := tracker.jobs["job-1"]
	tracker.mu.Unlock()
	assert.Equal(t, "/tmp/overwritten-again.mp4", state.chunks[0])
	assert.Len(t, state.chunks, 1)
}

func TestMultipleJobsTrackedIndependently(t *testing.T) {
	tracker := NewJobTracker()

	tracker.Add("job-A", 0, "/tmp/A-0.mp4", 2)
	tracker.Add("job-B", 0, "/tmp/B-0.mp4", 1)

	// job-B should complete independently of job-A
	readyB, chunksB := tracker.Add("job-B", 0, "/tmp/B-0.mp4", 1)
	assert.True(t, readyB)
	assert.NotNil(t, chunksB)

	// job-A still incomplete
	tracker.mu.Lock()
	_, existsA := tracker.jobs["job-A"]
	tracker.mu.Unlock()
	assert.True(t, existsA)
}

func TestConcurrentAccessNoDatabaseRace(t *testing.T) {
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

	// after all goroutines, job should be deleted (all chunks received)
	tracker.mu.Lock()
	_, exists := tracker.jobs["shared-job"]
	tracker.mu.Unlock()
	assert.False(t, exists, "job should be complete and deleted after all chunks received")
}

func TestConcurrentDifferentJobs(t *testing.T) {
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
}
