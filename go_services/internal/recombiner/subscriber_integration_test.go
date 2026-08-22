//go:build integration

package recombiner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"splice.com/go_services/internal/recombiner"
	shandler "splice.com/go_services/internal/shared/handler"
	"splice.com/go_services/internal/shared/test"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ackWaitI = 30 * time.Second

var sharedFilerURL string

func TestMain(m *testing.M) {
	filerURL, cleanup := test.StartSeaweedFSFiler()
	sharedFilerURL = filerURL

	code := m.Run()

	cleanup()
	os.Exit(code)
}

// it should create consumer with correct config
func TestReturnCorrectConfig(t *testing.T) {
	ctx := context.Background()
	js, _ := test.SetupNats(t)
	kv := test.SetupKV(t, js, "recombine-chunk-recieved")
	jobStatusKV := test.SetupJobStatusKV(t, js)
	claimKV := test.SetupKV(t, js, "recombine-chunk-claims")

	_, err := recombiner.RecombineVideo(js, kv, jobStatusKV, claimKV, ackWaitI, test.SilentLogger(), t.TempDir())
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
}

func TestMessageHandlingI(t *testing.T) {
	t.Run("invalid JSON does not publish downstream", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js, "recombine-chunk-recieved")
		jobStatusKV := test.SetupJobStatusKV(t, js)
		claimKV := test.SetupKV(t, js, "recombine-chunk-claims")

		_, err := recombiner.RecombineVideo(js, kv, jobStatusKV, claimKV, ackWaitI, test.SilentLogger(), t.TempDir())
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
		kv := test.SetupKV(t, js, "recombine-chunk-recieved")
		jobStatusKV := test.SetupJobStatusKV(t, js)
		claimKV := test.SetupKV(t, js, "recombine-chunk-claims")

		_, err := recombiner.RecombineVideo(js, kv, jobStatusKV, claimKV, ackWaitI, test.SilentLogger(), t.TempDir())
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			received <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		payload, err := json.Marshal(shandler.ChunkCompleteMessage{
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
		kv := test.SetupKV(t, js, "recombine-chunk-recieved")

		videoFile := test.OpenTestVideo(t, "../shared/test/testvideo.mp4")
		videoData, err := os.ReadFile(videoFile.Name())
		require.NoError(t, err)

		test.SeedProcessedVideo(t, sharedFilerURL, "job-combine", "chunk-0.mp4", videoData)
		test.SeedProcessedVideo(t, sharedFilerURL, "job-combine", "chunk-1.mp4", videoData)

		jobStatusKV := test.SetupJobStatusKV(t, js)
		claimKV := test.SetupKV(t, js, "recombine-chunk-claims")

		_, err = recombiner.RecombineVideo(js, kv, jobStatusKV, claimKV, ackWaitI, test.SilentLogger(), sharedFilerURL)
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			received <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		ctx := context.Background()
		for i, fileName := range []string{"chunk-0.mp4", "chunk-1.mp4"} {
			storageURL := fmt.Sprintf("%s/job-combine/processed/%s", sharedFilerURL, fileName)
			payload, err := json.Marshal(shandler.ChunkCompleteMessage{
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
		kv := test.SetupKV(t, js, "recombine-chunk-recieved")

		jobID := "job-idempotency-skip"

		// Pre-seed the KV as if this chunk was already received.
		_, err := kv.Put(context.Background(), fmt.Sprintf("%s.%d", jobID, 0), []byte("received"))
		require.NoError(t, err)

		jobStatusKV := test.SetupJobStatusKV(t, js)
		claimKV := test.SetupKV(t, js, "recombine-chunk-claims")

		_, err = recombiner.RecombineVideo(js, kv, jobStatusKV, claimKV, ackWaitI, test.SilentLogger(), sharedFilerURL)
		require.NoError(t, err)

		secondComplete := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			secondComplete <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		payload, err := json.Marshal(shandler.ChunkCompleteMessage{
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
		kv := test.SetupKV(t, js, "recombine-chunk-recieved")

		jobID := "job-idempotency-write"
		jobStatusKV := test.SetupJobStatusKV(t, js)
		claimKV := test.SetupKV(t, js, "recombine-chunk-claims")

		_, err := recombiner.RecombineVideo(js, kv, jobStatusKV, claimKV, ackWaitI, test.SilentLogger(), sharedFilerURL)
		require.NoError(t, err)

		// Partial chunk (TotalChunks:2) so combine never fires — KV write still happens after ack.
		payload, err := json.Marshal(shandler.ChunkCompleteMessage{
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

	t.Run("retry after transient combine failure still completes the job", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js, "recombine-chunk-recieved")

		jobID := "job-retry-after-fail"

		videoFile := test.OpenTestVideo(t, "../shared/test/testvideo.mp4")
		videoData, err := os.ReadFile(videoFile.Name())
		require.NoError(t, err)

		// chunk-0 is seeded up front; chunk-1 is NOT seeded yet, so its
		// download fails on the first attempt (simulating a transient
		// storage hiccup that later resolves).
		test.SeedProcessedVideo(t, sharedFilerURL, jobID, "chunk-0.mp4", videoData)

		jobStatusKV := test.SetupJobStatusKV(t, js)
		claimKV := test.SetupKV(t, js, "recombine-chunk-claims")

		_, err = recombiner.RecombineVideo(js, kv, jobStatusKV, claimKV, ackWaitI, test.SilentLogger(), sharedFilerURL)
		require.NoError(t, err)

		completed := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.complete", func(_ *nats.Msg) {
			completed <- struct{}{}
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		ctx := context.Background()
		chunk1URL := fmt.Sprintf("%s/%s/processed/chunk-1.mp4", sharedFilerURL, jobID)

		publishChunk := func(idx int, storageURL string) {
			payload, err := json.Marshal(shandler.ChunkCompleteMessage{
				JobID:       jobID,
				ChunkIndex:  idx,
				TotalChunks: 2,
				StorageURL:  storageURL,
			})
			require.NoError(t, err)
			_, err = js.Publish(ctx, "jobs.chunks.complete", payload)
			require.NoError(t, err)
		}

		publishChunk(0, fmt.Sprintf("%s/%s/processed/chunk-0.mp4", sharedFilerURL, jobID))
		publishChunk(1, chunk1URL)

		// First attempt fails to download chunk-1 since it isn't in storage yet.
		select {
		case <-completed:
			t.Fatal("job completed before chunk-1 was ever available in storage")
		case <-time.After(2 * time.Second):
		}

		// The transient failure resolves: chunk-1 becomes available and its
		// message is redelivered/retried.
		test.SeedProcessedVideo(t, sharedFilerURL, jobID, "chunk-1.mp4", videoData)
		publishChunk(1, chunk1URL)

		select {
		case <-completed:
		case <-time.After(30 * time.Second):
			t.Fatal("retry after transient failure never completed the job; chunk-1 was skipped as already received")
		}
	})
}
