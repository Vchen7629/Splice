//go:build integration

package main

import (
	"context"
	"encoding/json"
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

func TestQuitSignalExitsCleanly(t *testing.T) {
	js, nc := test.SetupNats(t)
	quit := make(chan os.Signal, 1)
	done := make(chan error, 1)

	go func() {
		done <- runCombiner(js, nc, test.SilentLogger(), t.TempDir(), quit)
	}()

	// give the consumer time to set up before signalling
	time.Sleep(200 * time.Millisecond)
	quit <- syscall.SIGTERM

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("runCombiner did not exit after quit signal")
	}
}

func TestFullFlowReceiveTranscodePublish(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	js, nc := test.SetupNats(t)

	// subscribe to the downstream subject before starting the worker
	received := make(chan []byte, 1)
	sub, err := nc.Subscribe("jobs.complete", func(msg *nats.Msg) {
		received <- msg.Data
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	outputDir := t.TempDir()
	quit := make(chan os.Signal, 1)
	done := make(chan error, 1)

	go func() {
		done <- runCombiner(js, nc, test.SilentLogger(), outputDir, quit)
	}()

	// give the consumer time to set up before publishing
	time.Sleep(500 * time.Millisecond)

	payload, err := json.Marshal(service.ChunkCompleteMessage{
		JobID:       "job-1",
		ChunkIndex:  0,
		TotalChunks: 1,
		OutputPath:  "idk",
	})
	require.NoError(t, err)

	_, err = js.Publish(context.Background(), "jobs.complete", payload)
	require.NoError(t, err)

	select {
	case data := <-received:
		var msg service.VideoProcessingCompleteMessage
		require.NoError(t, json.Unmarshal(data, &msg))
		assert.Equal(t, "job-1", msg.JobID)
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
}

func TestNoStreamReturnsError(t *testing.T) {
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
	err = runCombiner(js, nc, test.SilentLogger(), t.TempDir(), quit)

	assert.Error(t, err)
}
