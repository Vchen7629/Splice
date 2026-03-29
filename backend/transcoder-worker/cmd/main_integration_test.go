//go:build integration

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

func TestRunProcessingI(t *testing.T) {
	t.Run("quit signal exits cleanly", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)

		go func() {
			done <- runProcessing(js, nc, test.SilentLogger(), t.TempDir(), quit)
		}()

		// give the consumer time to set up before signalling
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

		// subscribe to the downstream subject before starting the worker
		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		outputDir := t.TempDir()
		quit := make(chan os.Signal, 1)
		done := make(chan error, 1)

		go func() {
			done <- runProcessing(js, nc, test.SilentLogger(), outputDir, quit)
		}()

		// give the consumer time to set up before publishing
		time.Sleep(500 * time.Millisecond)

		inputVideo := createTestVideo(t)
		payload, err := json.Marshal(service.VideoChunkMessage{
			JobID:            "job-1",
			ChunkIndex:       0,
			StoragePath:      inputVideo,
			TargetResolution: "240p",
		})
		require.NoError(t, err)

		_, err = js.Publish(context.Background(), "jobs.video.chunks", payload)
		require.NoError(t, err)

		select {
		case data := <-received:
			var msg service.ChunkCompleteMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, "job-1", msg.JobID)
			assert.Equal(t, 0, msg.ChunkIndex)
			assert.NotEmpty(t, msg.OutputPath)
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
		err = runProcessing(js, nc, test.SilentLogger(), t.TempDir(), quit)

		assert.Error(t, err)
	})
}

// createTestVideo generates a 1-second blue solid video using ffmpeg's lavfi source.
func createTestVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.mp4")
	cmd := exec.Command(
		"ffmpeg",
		"-f", "lavfi",
		"-i", "color=c=blue:s=320x240:d=1",
		"-c:v", "libx264",
		"-t", "1",
		"-y",
		path,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ffmpeg test video creation failed: %s", out)
	return path
}
