//go:build integration

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"pipeline-tests/helpers"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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
	baseURL, _ := helpers.SetupPipeline(t, 1, sharedFilerURL)

	t.Run("multi-chunk video is transcoded to target resolution", func(t *testing.T) {
		videoPath := filepath.Join(t.TempDir(), "test.mp4")
		helpers.GenerateTestVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		helpers.WaitForJobComplete(t, baseURL, jobID, 3*time.Minute)

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
		helpers.WaitForJobComplete(t, baseURL, jobID, 3*time.Minute)

		resp, err := helpers.DownloadVideo(t, baseURL, jobID, "output.mp4")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("job transitions from PROCESSING to COMPLETE", func(t *testing.T) {
		videoPath := filepath.Join(t.TempDir(), "test.mp4")
		helpers.GenerateTestVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		assert.Equal(t, "PROCESSING", helpers.PollJobStatus(t, baseURL, jobID))

		helpers.WaitForJobComplete(t, baseURL, jobID, 3*time.Minute)
		assert.Equal(t, "COMPLETE", helpers.PollJobStatus(t, baseURL, jobID))
	})
}

func TestFaultTolerance(t *testing.T) {
	t.Run("duplicate ChunkCompleteMessage does not trigger a second stitch", func(t *testing.T) {
		t.Skip("TODO: video-recombiner JobTracker does not deduplicate chunk indices")

		baseURL, natsURL := helpers.SetupPipeline(t, 1, sharedFilerURL)

		videoPath := filepath.Join(t.TempDir(), "test.mp4")
		helpers.GenerateTestVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		helpers.WaitForJobComplete(t, baseURL, jobID, 3*time.Minute)

		nc, err := nats.Connect(natsURL)
		require.NoError(t, err)
		t.Cleanup(nc.Close)

		js, err := jetstream.New(nc)
		require.NoError(t, err)

		secondComplete := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			secondComplete <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		// Re-publish a ChunkCompleteMessage for chunk 0 using its SeaweedFS storage URL.
		chunkStorageURL := fmt.Sprintf("%s/%s/chunk_000/processed", sharedFilerURL, jobID)
		payload, err := json.Marshal(struct {
			JobID       string `json:"job_id"`
			ChunkIndex  int    `json:"chunk_index"`
			TotalChunks int    `json:"total_chunks"`
			StorageURL  string `json:"storage_url"`
		}{
			JobID:       jobID,
			ChunkIndex:  0,
			TotalChunks: 1,
			StorageURL:  chunkStorageURL,
		})
		require.NoError(t, err)
		_, err = js.Publish(context.Background(), "jobs.chunks.complete", payload)
		require.NoError(t, err)

		select {
		case <-secondComplete:
			t.Fatal("duplicate ChunkCompleteMessage triggered a second stitch")
		case <-time.After(5 * time.Second):
		}
	})

	t.Run("redelivered SceneSplitMessage does not publish duplicate chunks", func(t *testing.T) {
		t.Skip("TODO: scene-detector has no idempotency check on redelivery")

		baseURL, natsURL := helpers.SetupPipeline(t, 1, sharedFilerURL)

		videoPath := filepath.Join(t.TempDir(), "test.mp4")
		helpers.GenerateTestVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		helpers.WaitForJobComplete(t, baseURL, jobID, 3*time.Minute)

		nc, err := nats.Connect(natsURL)
		require.NoError(t, err)
		t.Cleanup(nc.Close)

		js, err := jetstream.New(nc)
		require.NoError(t, err)

		secondComplete := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			secondComplete <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		// Re-publish the original SceneSplitMessage using its SeaweedFS storage URL.
		videoStorageURL := fmt.Sprintf("%s/%s/test.mp4", sharedFilerURL, jobID)
		payload, err := json.Marshal(struct {
			JobID            string `json:"job_id"`
			TargetResolution string `json:"target_resolution"`
			StorageURL       string `json:"storage_url"`
		}{
			JobID:            jobID,
			TargetResolution: "480p",
			StorageURL:       videoStorageURL,
		})
		require.NoError(t, err)
		_, err = js.Publish(context.Background(), "jobs.video.scene-split", payload)
		require.NoError(t, err)

		select {
		case <-secondComplete:
			t.Fatal("redelivered SceneSplitMessage caused a second pipeline run")
		case <-time.After(15 * time.Second):
		}
	})
}
