//go:build integration

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"pipeline-tests/helpers"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func TestTranscoderWorker(t *testing.T) {
	t.Run("redelivered VideoChunkMessage does not publish duplicate chunk complete", func(t *testing.T) {
		baseURL, statusURL, natsURL := helpers.SetupPipeline(t, 1, sharedFilerURL)

		nc, err := nats.Connect(natsURL)
		require.NoError(t, err)
		t.Cleanup(nc.Close)

		js, err := jetstream.New(nc)
		require.NoError(t, err)

		// Capture real VideoChunkMessages in flight before uploading so we have
		// valid storage URLs to replay after the job completes.
		capturedChunkMsgs := make(chan []byte, 10)
		captureSub, err := nc.Subscribe("jobs.video.chunks", func(m *nats.Msg) {
			capturedChunkMsgs <- m.Data
		})
		require.NoError(t, err)

		videoPath := filepath.Join(t.TempDir(), "test.mp4")
		helpers.GenerateTestVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		helpers.WaitForJobComplete(t, statusURL, jobID, 3*time.Minute)
		require.NoError(t, captureSub.Unsubscribe())

		// Now watch for any new jobs.chunks.complete after replaying the message.
		duplicateComplete := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(_ *nats.Msg) {
			duplicateComplete <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		// Replay the first captured VideoChunkMessage — storage URLs are real and
		// the file still exists in SeaweedFS, so the worker CAN process it.
		// Idempotency is the only thing that should prevent a second publish.
		payload := <-capturedChunkMsgs
		_, err = js.Publish(context.Background(), "jobs.video.chunks", payload)
		require.NoError(t, err)

		select {
		case <-duplicateComplete:
			t.Fatal("redelivered VideoChunkMessage caused a duplicate jobs.chunks.complete publish")
		case <-time.After(5 * time.Second):
		}
	})
}

func TestVideoRecombiner(t *testing.T) {
	t.Run("redelivered ChunkCompleteMessage does not trigger a second stitch", func(t *testing.T) {
		baseURL, statusURL, natsURL := helpers.SetupPipeline(t, 1, sharedFilerURL)

		nc, err := nats.Connect(natsURL)
		require.NoError(t, err)
		t.Cleanup(nc.Close)

		js, err := jetstream.New(nc)
		require.NoError(t, err)

		// Capture real ChunkCompleteMessages in flight before uploading so we have
		// valid storage URLs to replay after the job completes.
		capturedChunkCompleteMsgs := make(chan []byte, 10)
		captureSub, err := nc.Subscribe("jobs.chunks.complete", func(m *nats.Msg) {
			capturedChunkCompleteMsgs <- m.Data
		})
		require.NoError(t, err)

		videoPath := filepath.Join(t.TempDir(), "test.mp4")
		helpers.GenerateTestVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		helpers.WaitForJobComplete(t, statusURL, jobID, 3*time.Minute)
		require.NoError(t, captureSub.Unsubscribe())

		// Now watch for any new jobs.complete after replaying the message.
		secondComplete := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			secondComplete <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		// Replay the first captured ChunkCompleteMessage — storage URLs are real and
		// the file still exists in SeaweedFS, so the recombiner CAN stitch it.
		// Idempotency is the only thing that should prevent a second jobs.complete publish.
		payload := <-capturedChunkCompleteMsgs
		_, err = js.Publish(context.Background(), "jobs.chunks.complete", payload)
		require.NoError(t, err)

		select {
		case <-secondComplete:
			t.Fatal("redelivered ChunkCompleteMessage triggered a second stitch")
		case <-time.After(5 * time.Second):
		}
	})
}

func TestSceneDetectorFault(t *testing.T) {
	t.Run("duplicate ChunkCompleteMessage does not trigger a second stitch", func(t *testing.T) {
		baseURL, statusURL, natsURL := helpers.SetupPipeline(t, 1, sharedFilerURL)

		videoPath := filepath.Join(t.TempDir(), "test.mp4")
		helpers.GenerateTestVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		helpers.WaitForJobComplete(t, statusURL, jobID, 3*time.Minute)

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
		baseURL, statusURL, natsURL := helpers.SetupPipeline(t, 1, sharedFilerURL)

		videoPath := filepath.Join(t.TempDir(), "test.mp4")
		helpers.GenerateTestVideo(t, videoPath)

		jobID := helpers.UploadVideo(t, baseURL, videoPath, "480p")
		helpers.WaitForJobComplete(t, statusURL, jobID, 3*time.Minute)

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
