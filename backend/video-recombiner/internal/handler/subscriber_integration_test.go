//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"video-recombiner/internal/handler"
	"video-recombiner/internal/service"
	"video-recombiner/internal/test"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

func TestRecombineVideo(t *testing.T) {
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

		_, err = handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

		assert.Error(t, err)
	})

	t.Run("returns consume context", func(t *testing.T) {
		js, _ := test.SetupNats(t)

		consCtx, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
	})

	t.Run("creates consumer with correct config", func(t *testing.T) {
		ctx := context.Background()
		js, _ := test.SetupNats(t)

		_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())
		require.NoError(t, err)

		stream, err := js.Stream(ctx, "jobs")
		require.NoError(t, err)

		cons, err := stream.Consumer(ctx, "video-recombiner")
		require.NoError(t, err)

		info, err := cons.Info(ctx)
		require.NoError(t, err)

		assert.Equal(t, "video-recombiner", info.Config.Name)
		assert.Equal(t, "video-recombiner", info.Config.Durable)
		assert.Equal(t, "jobs.chunks.complete", info.Config.FilterSubject)
		assert.Equal(t, jetstream.AckExplicitPolicy, info.Config.AckPolicy)
		assert.Equal(t, 10, info.Config.MaxAckPending)
		assert.Equal(t, 3, info.Config.MaxDeliver)
		assert.Equal(t, 30*time.Second, info.Config.AckWait)
	})
}

func TestMessageHandlingI(t *testing.T) {
	t.Run("invalid JSON does not publish downstream", func(t *testing.T) {
		js, nc := test.SetupNats(t)

		_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			received <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		_, err = js.Publish(context.Background(), "jobs.chunks.complete", []byte("not valid json"))
		require.NoError(t, err)

		select {
		case <-received:
			t.Fatal("unexpected message published downstream after invalid JSON")
		case <-time.After(2 * time.Second):
		}
	})

	t.Run("partial chunk does not publish downstream", func(t *testing.T) {
		js, nc := test.SetupNats(t)

		_, err := handler.RecombineVideo(js, test.SilentLogger(), t.TempDir())
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			received <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		// Only the first of two chunks arrives — tracker not ready, no downstream publish.
		payload, err := json.Marshal(service.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 2,
			OutputPath:  "chunk-0.mp4",
		})
		require.NoError(t, err)

		_, err = js.Publish(context.Background(), "jobs.chunks.complete", payload)
		require.NoError(t, err)

		select {
		case <-received:
			t.Fatal("unexpected downstream publish after partial chunk")
		case <-time.After(2 * time.Second):
		}
	})

	t.Run("all chunks received triggers combine", func(t *testing.T) {
		outputDir := t.TempDir()
		js, nc := test.SetupNats(t)

		_, err := handler.RecombineVideo(js, test.SilentLogger(), outputDir)
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			received <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		ctx := context.Background()
		for i, path := range []string{"chunk-0.mp4", "chunk-1.mp4"} {
			payload, err := json.Marshal(service.ChunkCompleteMessage{
				JobID:       "job-1",
				ChunkIndex:  i,
				TotalChunks: 2,
				OutputPath:  path,
			})
			require.NoError(t, err)
			_, err = js.Publish(ctx, "jobs.chunks.complete", payload)
			require.NoError(t, err)
		}

		// Give the consumer time to process both messages and attempt combine.
		// Verified by manifest existence — ffmpeg will fail on fake paths.
		time.Sleep(2 * time.Second)

		manifest := filepath.Join(outputDir, "jobs", "job-1", "manifest.txt")
		_, err = os.Stat(manifest)
		assert.NoError(t, err, "manifest.txt should exist — confirms combine was triggered")

		select {
		case <-received:
			t.Fatal("unexpected downstream publish — ffmpeg should have failed on fake paths")
		case <-time.After(500 * time.Millisecond):
		}
	})
}
