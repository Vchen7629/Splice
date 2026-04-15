//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

var sharedFilerURL string

func TestMain(m *testing.M) {
	filerURL, cleanup := test.StartSeaweedFSFiler()
	sharedFilerURL = filerURL

	code := m.Run()

	cleanup()
	os.Exit(code)
}

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

		_, err = handler.RecombineVideo(js, nil, nil, test.SilentLogger(), t.TempDir())

		assert.Error(t, err)
	})

	t.Run("returns consume context", func(t *testing.T) {
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)
		jobStatusKV := test.SetupJobStatusKV(t, js)

		consCtx, err := handler.RecombineVideo(js, kv, jobStatusKV, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
	})

	t.Run("creates consumer with correct config", func(t *testing.T) {
		ctx := context.Background()
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)
		jobStatusKV := test.SetupJobStatusKV(t, js)

		_, err := handler.RecombineVideo(js, kv, jobStatusKV, test.SilentLogger(), t.TempDir())
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
		kv := test.SetupKV(t, js)
		jobStatusKV := test.SetupJobStatusKV(t, js)

		_, err := handler.RecombineVideo(js, kv, jobStatusKV, test.SilentLogger(), t.TempDir())
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
		kv := test.SetupKV(t, js)
		jobStatusKV := test.SetupJobStatusKV(t, js)

		_, err := handler.RecombineVideo(js, kv, jobStatusKV, test.SilentLogger(), t.TempDir())
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			received <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		payload, err := json.Marshal(service.ChunkCompleteMessage{
			JobID:       "job-partial",
			ChunkIndex:  0,
			TotalChunks: 2,
			StorageURL:  "http://storage/chunk-0.mp4",
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
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		videoFile := test.OpenTestVideo(t)
		videoData, err := os.ReadFile(videoFile.Name())
		require.NoError(t, err)

		test.SeedProcessedVideo(t, sharedFilerURL, "job-combine", "chunk-0.mp4", videoData)
		test.SeedProcessedVideo(t, sharedFilerURL, "job-combine", "chunk-1.mp4", videoData)

		jobStatusKV := test.SetupJobStatusKV(t, js)

		_, err = handler.RecombineVideo(js, kv, jobStatusKV, test.SilentLogger(), sharedFilerURL)
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			received <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		ctx := context.Background()
		for i, fileName := range []string{"chunk-0.mp4", "chunk-1.mp4"} {
			storageURL := fmt.Sprintf("%s/job-combine/%s/processed", sharedFilerURL, fileName)
			payload, err := json.Marshal(service.ChunkCompleteMessage{
				JobID:       "job-combine",
				ChunkIndex:  i,
				TotalChunks: 2,
				StorageURL:  storageURL,
			})
			require.NoError(t, err)
			_, err = js.Publish(ctx, "jobs.chunks.complete", payload)
			require.NoError(t, err)
		}

		select {
		case <-received:
		case <-time.After(30 * time.Second):
			t.Fatal("jobs.complete not published after all chunks received")
		}
	})
}

func TestRecombineVideoIdempotency(t *testing.T) {
	t.Run("already received chunk is acked and skipped", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		jobID := "job-idempotency-skip"

		// Pre-seed the KV as if this chunk was already received.
		_, err := kv.Put(context.Background(), fmt.Sprintf("%s.%d", jobID, 0), []byte("received"))
		require.NoError(t, err)

		jobStatusKV := test.SetupJobStatusKV(t, js)

		_, err = handler.RecombineVideo(js, kv, jobStatusKV, test.SilentLogger(), sharedFilerURL)
		require.NoError(t, err)

		secondComplete := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			secondComplete <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		payload, err := json.Marshal(service.ChunkCompleteMessage{
			JobID:       jobID,
			ChunkIndex:  0,
			TotalChunks: 1,
			StorageURL:  "http://storage/fake",
		})
		require.NoError(t, err)
		_, err = js.Publish(context.Background(), "jobs.chunks.complete", payload)
		require.NoError(t, err)

		select {
		case <-secondComplete:
			t.Fatal("already received chunk triggered a downstream publish")
		case <-time.After(2 * time.Second):
		}
	})

	t.Run("kv entry is written after chunk is acked", func(t *testing.T) {
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		jobID := "job-idempotency-write"
		jobStatusKV := test.SetupJobStatusKV(t, js)

		_, err := handler.RecombineVideo(js, kv, jobStatusKV, test.SilentLogger(), sharedFilerURL)
		require.NoError(t, err)

		// Partial chunk (TotalChunks:2) so combine never fires — KV write still happens after ack.
		payload, err := json.Marshal(service.ChunkCompleteMessage{
			JobID:       jobID,
			ChunkIndex:  0,
			TotalChunks: 2,
			StorageURL:  "http://storage/chunk-0.mp4",
		})
		require.NoError(t, err)
		_, err = js.Publish(context.Background(), "jobs.chunks.complete", payload)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			_, err := kv.Get(context.Background(), fmt.Sprintf("%s.%d", jobID, 0))
			return err == nil
		}, 10*time.Second, 200*time.Millisecond, "kv entry for received chunk was never written")
	})
}
