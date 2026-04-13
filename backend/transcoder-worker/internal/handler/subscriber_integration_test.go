//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

func TestConsumeVideoChunk(t *testing.T) {
	t.Run("no stream for subject returns error", func(t *testing.T) {
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
		kv := test.SetupKV(t, js)

		_, err = ConsumeVideoChunk(sharedFilerURL, js, kv, test.SilentLogger())

		assert.Error(t, err)
	})

	t.Run("returns non-nil consume context", func(t *testing.T) {
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		consCtx, err := ConsumeVideoChunk(sharedFilerURL, js, kv, test.SilentLogger())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
	})

	t.Run("consumer is created with correct config", func(t *testing.T) {
		ctx := context.Background()
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		_, err := ConsumeVideoChunk(sharedFilerURL, js, kv, test.SilentLogger())
		require.NoError(t, err)

		stream, err := js.Stream(ctx, "jobs")
		require.NoError(t, err)
		cons, err := stream.Consumer(ctx, "transcoder-worker")
		require.NoError(t, err)
		info, err := cons.Info(ctx)
		require.NoError(t, err)

		assert.Equal(t, "transcoder-worker", info.Config.Name)
		assert.Equal(t, "transcoder-worker", info.Config.Durable)
		assert.Equal(t, "jobs.video.chunks", info.Config.FilterSubject)
		assert.Equal(t, jetstream.AckExplicitPolicy, info.Config.AckPolicy)
		assert.Equal(t, 10, info.Config.MaxAckPending)
		assert.Equal(t, 3, info.Config.MaxDeliver)
	})

	t.Run("invalid JSON does not publish downstream", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		_, err := ConsumeVideoChunk(sharedFilerURL, js, kv, test.SilentLogger())
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(_ *nats.Msg) { received <- struct{}{} })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		invalidPayload, err := json.Marshal("not a VideoChunkMessage")
		require.NoError(t, err)
		_, err = js.Publish(context.Background(), "jobs.video.chunks", invalidPayload)
		require.NoError(t, err)

		select {
		case <-received:
			t.Fatal("unexpected message published downstream after invalid JSON")
		case <-time.After(2 * time.Second):
		}
	})

	t.Run("valid message publishes chunk complete and acks", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		jobID := "job-full-flow"
		t.Cleanup(func() {
			os.RemoveAll("/tmp/temp-unprocessed-" + jobID)
			os.RemoveAll("/tmp/temp-processed-" + jobID)
		})

		videoContent, err := os.ReadFile("../test/test_video.mp4")
		require.NoError(t, err)
		storageURL := test.SeedUnprocessedVideo(t, sharedFilerURL, jobID, "test_video.mp4", videoContent)

		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(m *nats.Msg) { received <- m.Data })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		_, err = ConsumeVideoChunk(sharedFilerURL, js, kv, test.SilentLogger())
		require.NoError(t, err)

		test.PublishVideoChunk(t, js, service.VideoChunkMessage{
			JobID: jobID, ChunkIndex: 0, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p",
		})

		select {
		case data := <-received:
			var msg service.ChunkCompleteMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, jobID, msg.JobID)
			assert.Equal(t, 0, msg.ChunkIndex)
			assert.Equal(t, 1, msg.TotalChunks)
			assert.Equal(t, fmt.Sprintf("%s/%s/processed/test_video.mp4", sharedFilerURL, jobID), msg.StorageURL)
		case <-time.After(30 * time.Second):
			t.Fatal("timed out waiting for chunk complete message")
		}
	})
}

func TestConsumeVideoChunkNaksOnError(t *testing.T) {
	tests := []struct {
		name           string
		baseStorageURL string
		videoContent   func(t *testing.T) []byte
		fileName       string
	}{
		{
			name:           "transcode failure naks for redelivery",
			baseStorageURL: sharedFilerURL,
			videoContent:   func(_ *testing.T) []byte { return []byte("this is not a video") },
			fileName:       "not_a_video.mp4",
		},
		{
			name:           "save failure naks for redelivery",
			baseStorageURL: "http://localhost:1",
			videoContent: func(t *testing.T) []byte {
				b, err := os.ReadFile("../test/test_video.mp4")
				require.NoError(t, err)
				return b
			},
			fileName: "test_video.mp4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			js, _ := test.SetupNats(t)
			kv := test.SetupKV(t, js)
			jobID := "job-nak-" + tc.fileName
			t.Cleanup(func() {
				os.RemoveAll("/tmp/temp-unprocessed-" + jobID)
				os.RemoveAll("/tmp/temp-processed-" + jobID)
			})

			storageURL := test.SeedUnprocessedVideo(t, sharedFilerURL, jobID, tc.fileName, tc.videoContent(t))

			_, err := ConsumeVideoChunk(tc.baseStorageURL, js, kv, test.SilentLogger())
			require.NoError(t, err)

			test.PublishVideoChunk(t, js, service.VideoChunkMessage{
				JobID: jobID, ChunkIndex: 0, TotalChunks: 1,
				StorageURL: storageURL, TargetResolution: "480p",
			})

			test.AssertNacked(t, js, "expected message to be nacked")
		})
	}
}

func TestConsumeVideoChunkPublishFails(t *testing.T) {
	t.Run("publish error naks the message for redelivery", func(t *testing.T) {
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

		// Stream only covers input subject — publish to jobs.chunks.complete will error
		_, err = js.CreateStream(ctx, jetstream.StreamConfig{
			Name:     "jobs",
			Subjects: []string{"jobs.video.chunks"},
		})
		require.NoError(t, err)

		kv := test.SetupKV(t, js)

		jobID := "job-publish-fail"
		t.Cleanup(func() {
			os.RemoveAll("/tmp/temp-unprocessed-" + jobID)
			os.RemoveAll("/tmp/temp-processed-" + jobID)
		})

		videoContent, err := os.ReadFile("../test/test_video.mp4")
		require.NoError(t, err)
		storageURL := test.SeedUnprocessedVideo(t, sharedFilerURL, jobID, "test_video.mp4", videoContent)

		_, err = ConsumeVideoChunk(sharedFilerURL, js, kv, test.SilentLogger())
		require.NoError(t, err)

		test.PublishVideoChunk(t, js, service.VideoChunkMessage{
			JobID: jobID, ChunkIndex: 0, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p",
		})

		test.AssertNacked(t, js, "expected message to be nacked after publish failure")
	})
}

func TestConsumeVideoChunkCleanup(t *testing.T) {
	seedAndConsume := func(t *testing.T, jobID string) (jetstream.JetStream, <-chan struct{}) {
		t.Helper()
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		videoContent, err := os.ReadFile("../test/test_video.mp4")
		require.NoError(t, err)
		storageURL := test.SeedUnprocessedVideo(t, sharedFilerURL, jobID, "test_video.mp4", videoContent)

		_, err = ConsumeVideoChunk(sharedFilerURL, js, kv, test.SilentLogger())
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(_ *nats.Msg) { received <- struct{}{} })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		test.PublishVideoChunk(t, js, service.VideoChunkMessage{
			JobID: jobID, ChunkIndex: 0, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p",
		})

		return js, received
	}

	tests := []struct {
		name       string
		jobID      string
		failOnCall int
	}{
		{"removeAll error on unprocessed dir logs warn and returns", "job-cleanup-unprocessed-err", 1},
		{"removeAll error on processed dir logs warn and returns", "job-cleanup-processed-err", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				removeAll = os.RemoveAll
				os.RemoveAll("/tmp/temp-unprocessed-" + tc.jobID)
				os.RemoveAll("/tmp/temp-processed-" + tc.jobID)
			})

			calls := 0
			removeAll = func(path string) error {
				calls++
				if calls == tc.failOnCall {
					return errors.New("remove failed")
				}
				return os.RemoveAll(path)
			}

			_, received := seedAndConsume(t, tc.jobID)

			select {
			case <-received:
			case <-time.After(30 * time.Second):
				t.Fatal("timed out waiting for chunk complete message")
			}
			time.Sleep(500 * time.Millisecond)
			assert.Equal(t, tc.failOnCall, calls)
		})
	}
}

func TestConsumeVideoChunkIdempotency(t *testing.T) {
	t.Run("already processed chunk is acked and skipped", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		jobID := "job-idempotency-skip"

		// Pre-seed the KV as if this chunk was already processed.
		_, err := kv.Put(context.Background(), fmt.Sprintf("%s.%d", jobID, 0), []byte("processed"))
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(_ *nats.Msg) { received <- struct{}{} })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		_, err = ConsumeVideoChunk(sharedFilerURL, js, kv, test.SilentLogger())
		require.NoError(t, err)

		test.PublishVideoChunk(t, js, service.VideoChunkMessage{
			JobID: jobID, ChunkIndex: 0, TotalChunks: 1,
			StorageURL: "http://storage/fake", TargetResolution: "480p",
		})

		select {
		case <-received:
			t.Fatal("already processed chunk triggered a downstream publish")
		case <-time.After(2 * time.Second):
		}
	})

	t.Run("kv entry is written after successful processing", func(t *testing.T) {
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		jobID := "job-idempotency-write"
		t.Cleanup(func() {
			os.RemoveAll("/tmp/temp-unprocessed-" + jobID)
			os.RemoveAll("/tmp/temp-processed-" + jobID)
		})

		videoContent, err := os.ReadFile("../test/test_video.mp4")
		require.NoError(t, err)
		storageURL := test.SeedUnprocessedVideo(t, sharedFilerURL, jobID, "test_video.mp4", videoContent)

		_, err = ConsumeVideoChunk(sharedFilerURL, js, kv, test.SilentLogger())
		require.NoError(t, err)

		test.PublishVideoChunk(t, js, service.VideoChunkMessage{
			JobID: jobID, ChunkIndex: 0, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p",
		})

		// Wait for processing to complete then verify KV entry exists.
		require.Eventually(t, func() bool {
			_, err := kv.Get(context.Background(), fmt.Sprintf("%s.%d", jobID, 0))
			return err == nil
		}, 30*time.Second, 200*time.Millisecond, "kv entry for processed chunk was never written")
	})

	t.Run("kv entry is not written when processing fails", func(t *testing.T) {
		js, _ := test.SetupNats(t)
		kv := test.SetupKV(t, js)

		jobID := "job-idempotency-no-write-on-fail"
		t.Cleanup(func() {
			os.RemoveAll("/tmp/temp-unprocessed-" + jobID)
			os.RemoveAll("/tmp/temp-processed-" + jobID)
		})

		// Seed invalid video so transcoding fails.
		storageURL := test.SeedUnprocessedVideo(t, sharedFilerURL, jobID, "bad.mp4", []byte("not a video"))

		_, err := ConsumeVideoChunk(sharedFilerURL, js, kv, test.SilentLogger())
		require.NoError(t, err)

		test.PublishVideoChunk(t, js, service.VideoChunkMessage{
			JobID: jobID, ChunkIndex: 0, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p",
		})

		time.Sleep(2 * time.Second)

		_, err = kv.Get(context.Background(), fmt.Sprintf("%s.%d", jobID, 0))
		assert.ErrorIs(t, err, jetstream.ErrKeyNotFound, "kv entry should not exist after failed processing")
	})
}
