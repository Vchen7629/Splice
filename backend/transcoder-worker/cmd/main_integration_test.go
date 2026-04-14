//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
	"transcoder-worker/internal/service"
	"transcoder-worker/internal/test"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

var sharedFilerURL string

func TestMain(m *testing.M) {
	filerURL, cleanup := test.StartSeaweedFSFiler()
	sharedFilerURL = filerURL

	code := m.Run()

	cleanup()
	os.Exit(code)
}

func TestRunProcessingI(t *testing.T) {
	t.Run("quit signal exits cleanly", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js)
		jobStatusKV := test.SetupJobStatusKV(t, js)
		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)

		go func() {
			done <- runProcessing(sharedFilerURL, "0", kv, jobStatusKV, js, nc, test.SilentLogger(), quit)
		}()

		time.Sleep(200 * time.Millisecond)
		quit <- syscall.SIGTERM

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("runProcessing did not exit after quit signal")
		}
	})

	t.Run("full flow receives transcode message and publishes downstream", func(t *testing.T) {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			t.Skip("ffmpeg not available")
		}

		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		jobID := "job-full-flow"
		t.Cleanup(func() {
			os.RemoveAll("/tmp/temp-unprocessed-" + jobID)
			os.RemoveAll("/tmp/temp-processed-" + jobID)
		})

		videoContent, err := os.ReadFile("../internal/test/test_video.mp4")
		require.NoError(t, err)
		storageURL := test.SeedUnprocessedVideo(t, sharedFilerURL, jobID, "test_video.mp4", videoContent)

		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)
		jobStatusKV := test.SetupJobStatusKV(t, js)

		go func() {
			done <- runProcessing(sharedFilerURL, "0", kv, jobStatusKV, js, nc, test.SilentLogger(), quit)
		}()

		time.Sleep(500 * time.Millisecond)

		payload, err := json.Marshal(service.VideoChunkMessage{
			JobID:            jobID,
			ChunkIndex:       0,
			TotalChunks:      1,
			StorageURL:       storageURL,
			TargetResolution: "240p",
		})
		require.NoError(t, err)

		_, err = js.Publish(context.Background(), "jobs.video.chunks", payload)
		require.NoError(t, err)

		select {
		case data := <-received:
			var msg service.ChunkCompleteMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, jobID, msg.JobID)
			assert.Equal(t, 0, msg.ChunkIndex)
			assert.Equal(t, fmt.Sprintf("%s/%s/processed/test_video.mp4", sharedFilerURL, jobID), msg.StorageURL)
		case <-time.After(30 * time.Second):
			t.Fatal("timed out waiting for downstream message")
		}

		quit <- syscall.SIGTERM

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("runProcessing did not exit after quit signal")
		}
	})

	t.Run("no stream returns error", func(t *testing.T) {
		ctx := context.Background()

		container, err := natstc.Run(ctx, "nats:2.10-alpine")
		require.NoError(t, err)
		t.Cleanup(func() { _ = container.Terminate(ctx) })

		url, err := container.ConnectionString(ctx)
		require.NoError(t, err)

		nc, err := nats.Connect(url)
		require.NoError(t, err)
		t.Cleanup(nc.Close)

		js, err := jetstream.New(nc)
		require.NoError(t, err)

		quit := make(chan os.Signal, 1)
		jobStatusKV := test.SetupJobStatusKV(t, js)

		err = runProcessing(sharedFilerURL, "0", &test.MockKV{}, jobStatusKV, js, nc, test.SilentLogger(), quit)

		assert.Error(t, err)
	})
}

func TestKVSetup(t *testing.T) {
	t.Run("CreateOrUpdateKeyValue fails when JetStream is not enabled", func(t *testing.T) {
		nc := test.SetupNatsNoJetStream(t)

		js, err := jetstream.New(nc)
		require.NoError(t, err)

		_, err = js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{Bucket: "transcode-chunk-job-processed"})

		assert.Error(t, err)
	})
}

func TestMainI(t *testing.T) {
	t.Run("exits on NATS connect error", func(t *testing.T) {
		code := patchOsExit(t)
		writeEnvFile(t, fmt.Sprintf("BASE_STORAGE_URL=%s\nNATS_URL=nats://localhost:1\nHTTP_PORT=0\n", sharedFilerURL))

		main()

		assert.Equal(t, 1, *code)
	})

	t.Run("reaches runProcessing and logs error on no stream", func(t *testing.T) {
		ctx := context.Background()
		container, err := natstc.Run(ctx, "nats:2.10-alpine")
		require.NoError(t, err)
		t.Cleanup(func() { _ = container.Terminate(ctx) })

		natsURL, err := container.ConnectionString(ctx)
		require.NoError(t, err)

		// Pre-create job-status bucket (video-status would have done this in prod).
		// No stream configured — ConsumeVideoChunk fails, main() logs error and returns without osExit.
		setupNC, err := nats.Connect(natsURL)
		require.NoError(t, err)
		defer setupNC.Close()
		setupJS, err := jetstream.New(setupNC)
		require.NoError(t, err)
		test.SetupJobStatusKV(t, setupJS)

		code := patchOsExit(t)
		writeEnvFile(t, fmt.Sprintf("BASE_STORAGE_URL=%s\nNATS_URL=%s\nHTTP_PORT=0\n", sharedFilerURL, natsURL))

		main()

		assert.Equal(t, -1, *code) // osExit was never called
	})
}
