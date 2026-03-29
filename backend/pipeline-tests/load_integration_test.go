//go:build integration

package e2e

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConcurrentJobs(t *testing.T) {
	const numJobs = 5
	const numWorkers = 3

	baseURL, _, _ := setupPipeline(t, numWorkers)

	jobIDs := make([]string, numJobs)
	var wg sync.WaitGroup

	for i := range numJobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			videoPath := filepath.Join(t.TempDir(), "test.mp4")
			generateTestVideo(t, videoPath)
			jobIDs[i] = uploadVideo(t, baseURL, videoPath, "480p")
		}()
	}
	wg.Wait()

	for _, id := range jobIDs {
		waitForJobComplete(t, baseURL, id, 5*time.Minute)
		assert.Equal(t, "COMPLETE", pollJobStatus(t, baseURL, id))
	}
}
