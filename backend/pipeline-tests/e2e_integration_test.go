//go:build integration

package e2e

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"pipeline-tests/helpers"
	"testing"
	"time"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sharedFilerURL string

func TestMain(m *testing.M) {
	filerURL, cleanup := helpers.StartSeaweedFSFiler()
	sharedFilerURL = filerURL

	code := m.Run()

	cleanup()
	os.Exit(code)
}

func TestPipelineHappyPath(t *testing.T) {
	baseURL, statusURL, _ := helpers.SetupPipeline(t, 1, sharedFilerURL)

	t.Run("multi-chunk video is transcoded to target resolution", func(t *testing.T) {
		videoPath := filepath.Join(t.TempDir(), "test.mp4")
		helpers.GenerateTestVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		helpers.WaitForJobComplete(t, statusURL, jobID, 3*time.Minute)

		resp, err := helpers.DownloadVideo(t, baseURL, jobID, "output.mp4")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		outputPath := filepath.Join(t.TempDir(), "output.mp4")
		f, err := os.Create(outputPath)
		require.NoError(t, err)
		defer f.Close()
		_, err = io.Copy(f, resp.Body)
		require.NoError(t, err)
		f.Close()

		out, err := exec.Command("ffprobe",
			"-v", "error",
			"-select_streams", "v:0",
			"-show_entries", "stream=height",
			"-of", "default=noprint_wrappers=1:nokey=1",
			outputPath,
		).CombinedOutput()
		require.NoError(t, err, "ffprobe failed:\n%s", out)
		assert.Equal(t, "480\n", string(out))
	})

	t.Run("single-chunk video with no scene boundary completes successfully", func(t *testing.T) {
		videoPath := filepath.Join(t.TempDir(), "single.mp4")
		helpers.GenerateSingleSceneVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		helpers.WaitForJobComplete(t, statusURL, jobID, 3*time.Minute)

		resp, err := helpers.DownloadVideo(t, baseURL, jobID, "output.mp4")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("job transitions from PROCESSING to COMPLETE", func(t *testing.T) {
		videoPath := filepath.Join(t.TempDir(), "test.mp4")
		helpers.GenerateTestVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		assert.Equal(t, "PROCESSING", helpers.PollJobStatus(t, statusURL, jobID))

		helpers.WaitForJobComplete(t, statusURL, jobID, 3*time.Minute)
		assert.Equal(t, "COMPLETE", helpers.PollJobStatus(t, statusURL, jobID))
	})
}