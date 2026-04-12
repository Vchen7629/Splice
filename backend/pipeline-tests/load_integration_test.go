//go:build integration

package e2e

import (
	"path/filepath"
	"pipeline-tests/helpers"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConcurrentJobs(t *testing.T) {
	const numJobs = 5
	const numWorkers = 3

	baseURL, statusURL, _ := helpers.SetupPipeline(t, numWorkers, sharedFilerURL)

	jobIDs := make([]string, numJobs)
	var wg sync.WaitGroup

	for i := range numJobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			videoPath := filepath.Join(t.TempDir(), "test.mp4")
			helpers.GenerateTestVideo(t, videoPath)
			jobIDs[i] = helpers.UploadVideo(t, baseURL, videoPath, "480p")
		}()
	}
	wg.Wait()

	for _, id := range jobIDs {
		helpers.WaitForJobComplete(t, statusURL, id, 5*time.Minute)
		assert.Equal(t, "COMPLETE", helpers.PollJobStatus(t, statusURL, id))
	}
}
