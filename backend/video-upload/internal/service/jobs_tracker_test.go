// go:build test

package service_test

import (
	"fmt"
	"sync"
	"testing"
	"video-upload/internal/service"

	"github.com/stretchr/testify/assert"
)

func TestAddSeenVisitor(t *testing.T) {
	clearSeenJobs := service.NewCompletedJobs().ClearAllJobs
	completedJobs := service.NewCompletedJobs()

	t.Run("Adding one jobID", func(t *testing.T) {
		t.Cleanup(clearSeenJobs)

		completedJobs.AddJob("job-1")

		assert.True(t, completedJobs.IsDone("job-1"))
	})

	t.Run("Adding the same jobID twice does not cause errors", func(t *testing.T) {
		t.Cleanup(clearSeenJobs)

		completedJobs.AddJob("job-1")
		completedJobs.AddJob("job-1")

		assert.True(t, completedJobs.IsDone("job-1"))
	})

	t.Run("Concurrent adds do not produce data races", func(t *testing.T) {
		t.Cleanup(clearSeenJobs)

		const goroutines = 50
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for i := range goroutines {
			go func(id string) {
				defer wg.Done()
				completedJobs.AddJob(id)
			}(fmt.Sprintf("job-%d", i))
		}

		wg.Wait()

		for i := range goroutines {
			assert.True(t, completedJobs.IsDone(fmt.Sprintf("job-%d", i)), "jobID %d should be marked as seen", i)
		}
	})
}

func TestHasSeenVisitor(t *testing.T) {
	clearSeenJobs := service.NewCompletedJobs().ClearAllJobs
	completedJobs := service.NewCompletedJobs()

	t.Run("Returns false for jobID that has not been added", func(t *testing.T) {
		t.Cleanup(clearSeenJobs)

		assert.False(t, completedJobs.IsDone("idk"))
	})

	t.Run("Returns true for jobID that has been added", func(t *testing.T) {
		t.Cleanup(clearSeenJobs)

		completedJobs.AddJob("job-1")

		assert.True(t, completedJobs.IsDone("job-1"))
	})

	t.Run("Concurrent reads do not produce data races", func(t *testing.T) {
		t.Cleanup(clearSeenJobs)
		completedJobs.AddJob("job-1")

		const goroutines = 50
		var wg sync.WaitGroup
		wg.Add(goroutines)

		for range goroutines {
			go func() {
				defer wg.Done()
				completedJobs.IsDone("job-1")
			}()
		}

		wg.Wait()
	})
}

// cross-function tests for concurrent access across Add, Get, Has, and Clear
func TestSeenFaceIds_CrossFunction(t *testing.T) {
	clearSeenJobs := service.NewCompletedJobs().ClearAllJobs
	completedJobs := service.NewCompletedJobs()

	t.Run("Concurrent AddJob and IsDone do not produce data races", func(t *testing.T) {
		t.Cleanup(clearSeenJobs)

		const goroutines = 50
		var wg sync.WaitGroup
		wg.Add(goroutines * 2)

		for i := range goroutines {
			go func(id string) {
				defer wg.Done()
				completedJobs.AddJob(id)
			}(fmt.Sprintf("job-%d", i))

			go func(id string) {
				defer wg.Done()
				completedJobs.IsDone(id)
			}(fmt.Sprintf("job-%d", i))
		}

		wg.Wait()
	})
}