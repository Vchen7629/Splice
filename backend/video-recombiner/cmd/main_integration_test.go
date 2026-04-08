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
	"video-recombiner/internal/service"
	"video-recombiner/internal/test"

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

func TestRunCombinerI(t *testing.T) {
	t.Run("quit signal exits cleanly", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)

		go func() {
			done <- runCombiner(js, nc, test.SilentLogger(), sharedFilerURL, quit)
		}()

		time.Sleep(200 * time.Millisecond)
		quit <- syscall.SIGTERM

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("runCombiner did not exit after quit signal")
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
		err = runCombiner(js, nc, test.SilentLogger(), sharedFilerURL, quit)

		assert.Error(t, err)
	})

	t.Run("full flow: receive chunks, combine, publish downstream", func(t *testing.T) {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			t.Skip("ffmpeg not available")
		}

		jobID := "job-full-flow"
		t.Cleanup(func() {
			os.RemoveAll("/tmp/processed_chunk-" + jobID)
			os.RemoveAll("/tmp/jobs/" + jobID)
		})

		videoData, err := os.ReadFile("../internal/test/testvideo.mp4")
		require.NoError(t, err)

		test.SeedProcessedVideo(t, sharedFilerURL, jobID, "chunk-0.mp4", videoData)
		test.SeedProcessedVideo(t, sharedFilerURL, jobID, "chunk-1.mp4", videoData)

		js, nc := test.SetupNats(t)

		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.complete", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)

		go func() {
			done <- runCombiner(js, nc, test.SilentLogger(), sharedFilerURL, quit)
		}()

		time.Sleep(500 * time.Millisecond)

		ctx := context.Background()
		for i, fileName := range []string{"chunk-0.mp4", "chunk-1.mp4"} {
			storageURL := fmt.Sprintf("%s/%s/%s/processed", sharedFilerURL, jobID, fileName)
			payload, err := json.Marshal(service.ChunkCompleteMessage{
				JobID:       jobID,
				ChunkIndex:  i,
				TotalChunks: 2,
				StorageURL:  storageURL,
			})
			require.NoError(t, err)
			_, err = js.Publish(ctx, "jobs.chunks.complete", payload)
			require.NoError(t, err)
		}

		select {
		case data := <-received:
			var msg service.VideoProcessingCompleteMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, jobID, msg.JobID)
		case <-time.After(30 * time.Second):
			t.Fatal("timed out waiting for downstream message")
		}

		quit <- syscall.SIGTERM

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("runCombiner did not exit after quit signal")
		}
	})
}

func TestMainI(t *testing.T) {
	t.Run("storage unreachable exits with code 1", func(t *testing.T) {
		code := patchExit(t)
		writeEnvFile(t, "BASE_STORAGE_URL=http://localhost:1\nNATS_URL=nats://localhost:4222\n")

		main()

		assert.Equal(t, 1, *code)
	})

	t.Run("nats unreachable exits with code 1", func(t *testing.T) {
		code := patchExit(t)
		writeEnvFile(t, fmt.Sprintf("BASE_STORAGE_URL=%s\nNATS_URL=nats://localhost:1\n", sharedFilerURL))

		main()

		assert.Equal(t, 1, *code)
	})

	t.Run("no stream logs error and returns", func(t *testing.T) {
		ctx := context.Background()
		container, err := natstc.Run(ctx, "nats:2.10-alpine")
		require.NoError(t, err)
		t.Cleanup(func() { _ = container.Terminate(ctx) })

		natsURL, err := container.ConnectionString(ctx)
		require.NoError(t, err)

		code := patchExit(t)
		writeEnvFile(t, fmt.Sprintf("BASE_STORAGE_URL=%s\nNATS_URL=%s\n", sharedFilerURL, natsURL))

		main()

		assert.Equal(t, -1, *code) // osExit was never called
	})
}
