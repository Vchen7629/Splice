//go:build integration

package chunkworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/test"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

// helpers

func publishVideoChunk(t *testing.T, js jetstream.JetStream, msg pipeline.VideoChunkMessage) {
	t.Helper()
	payload, err := json.Marshal(msg)
	require.NoError(t, err)
	_, err = js.Publish(context.Background(), "jobs.video.chunks", payload)
	require.NoError(t, err)
}

// assertNacked polls until the named consumer has redelivered the message, confirming it was nacked.
func assertNacked(t *testing.T, js jetstream.JetStream, msg string) {
	t.Helper()
	ctx := context.Background()
	require.Eventually(t, func() bool {
		stream, err := js.Stream(ctx, "jobs")
		if err != nil {
			return false
		}
		cons, err := stream.Consumer(ctx, "chunk-worker")
		if err != nil {
			return false
		}
		info, err := cons.Info(ctx)
		if err != nil {
			return false
		}
		return info.NumRedelivered > 0
	}, 30*time.Second, 200*time.Millisecond, msg)
}

func cleanupTestTempFolders(t *testing.T, chunkName string) {
	t.Helper()
	dirs := []string{
		"/tmp/transcode/temp-processed-" + chunkName,
		"/tmp/denoise/temp-processed-" + chunkName,
	}
	t.Cleanup(func() {
		for _, d := range dirs {
			_ = os.RemoveAll(d)
		}
	})
}

var sharedFilerURL string

func TestMain(m *testing.M) {
	filerURL, cleanup := test.StartSeaweedFSFiler()
	sharedFilerURL = filerURL

	code := m.Run()

	cleanup()
	os.Exit(code)
}

func TestConsumeVideoChunkI(t *testing.T) {
	t.Run("consumer is created with correct config", func(t *testing.T) {
		ctx := context.Background()
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js, "chunk-processed")
		claimKV := test.SetupKV(t, js, "chunk-claims")
		jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

		_, err := ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, 30*time.Second, test.SilentLogger())
		require.NoError(t, err)

		stream, err := js.Stream(ctx, "jobs")
		require.NoError(t, err)
		cons, err := stream.Consumer(ctx, "chunk-worker")
		require.NoError(t, err)
		info, err := cons.Info(ctx)
		require.NoError(t, err)

		assert.Equal(t, "chunk-worker", info.Config.Name)
		assert.Equal(t, "jobs.video.chunks", info.Config.FilterSubject)
		assert.Equal(t, 10, info.Config.MaxAckPending)
	})

	t.Run("invalid JSON does not publish downstream", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js, "chunk-processed")
		claimKV := test.SetupKV(t, js, "chunk-claims")
		jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

		_, err := ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, 30*time.Second, test.SilentLogger())
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(_ *nats.Msg) { received <- struct{}{} })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		invalidPayload, err := json.Marshal("not a pipeline.VideoChunkMessage")
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
		kv := test.SetupKV(t, js, "chunk-processed")

		jobID := "job-full-flow"
		cleanupTestTempFolders(t, jobID+"-0")

		storageURL := test.ServeTestChunks(t, 1) + "/chunk.mkv"

		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(m *nats.Msg) { received <- m.Data })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })
		claimKV := test.SetupKV(t, js, "chunk-claims")
		jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

		_, err = ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, 30*time.Second, test.SilentLogger())
		require.NoError(t, err)

		publishVideoChunk(t, js, pipeline.VideoChunkMessage{
			ChunkRef: pipeline.ChunkRef{JobID: jobID, ChunkIndex: 0}, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p", Operation: "transcode",
		})

		select {
		case data := <-received:
			var msg pipeline.ChunkCompleteMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, jobID, msg.JobID)
			assert.Equal(t, 0, msg.ChunkIndex)
			assert.Equal(t, 1, msg.TotalChunks)
			assert.Equal(t, fmt.Sprintf("%s/%s/processed/%s-0.mp4", sharedFilerURL, jobID, jobID), msg.StorageURL)
		case <-time.After(30 * time.Second):
			t.Fatal("timed out waiting for chunk complete message")
		}
	})
}

func TestConsumeVideoChunkPublishesProgress(t *testing.T) {
	js, nc := test.SetupNats(t)
	kv := test.SetupKV(t, js, "chunk-processed")
	claimKV := test.SetupKV(t, js, "chunk-claims")
	jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

	jobID := "job-progress"
	cleanupTestTempFolders(t, jobID+"-0")
	cleanupTestTempFolders(t, jobID+"-1")

	storageURL0 := test.ServeTestChunks(t, 5) + "/chunk.mkv" // slow
	storageURL1 := test.ServeTestChunks(t, 1) + "/chunk.mkv" // fast

	// simulate 2 competing workers on same durable consumer
	for range 2 {
		_, err := ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, 30*time.Second, test.SilentLogger())
		require.NoError(t, err)
	}

	var mu sync.Mutex
	var percents []int
	reached100 := make(chan struct{}, 1)
	progressSub, err := nc.Subscribe(fmt.Sprintf("progress.%s", jobID), func(m *nats.Msg) {
		var progress pipeline.ProgressMessage
		if json.Unmarshal(m.Data, &progress) != nil {
			return
		}
		if progress.Stage != "chunk-worker" {
			t.Errorf("expected stage chunk-worker, got %q", progress.Stage)
		}
		mu.Lock()
		percents = append(percents, progress.Progress)
		mu.Unlock()
		if progress.Progress == 100 {
			reached100 <- struct{}{}
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = progressSub.Unsubscribe() })

	publishVideoChunk(t, js, pipeline.VideoChunkMessage{
		ChunkRef: pipeline.ChunkRef{JobID: jobID, ChunkIndex: 0}, TotalChunks: 2,
		StorageURL: storageURL0, TargetResolution: "480p", Operation: "transcode",
	})
	publishVideoChunk(t, js, pipeline.VideoChunkMessage{
		ChunkRef: pipeline.ChunkRef{JobID: jobID, ChunkIndex: 1}, TotalChunks: 2,
		StorageURL: storageURL1, TargetResolution: "480p", Operation: "transcode",
	})

	select {
	case <-reached100:
	case <-time.After(30 * time.Second):
		t.Fatalf("progress.%s never reached 100", jobID)
	}

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, sort.IntsAreSorted(percents), "progress must be monotonically non-decreasing under concurrent chunk completion, got %v", percents)
}

func TestConsumeVideoChunk_RetryAfterCompletionDoesNotRegressMilestone(t *testing.T) {
	js, nc := test.SetupNats(t)
	kv := test.SetupKV(t, js, "chunk-processed")
	claimKV := test.SetupKV(t, js, "chunk-claims")
	jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

	jobID := "job-late-chunk-after-complete"
	cleanupTestTempFolders(t, jobID+"-0")

	completeStatus, err := json.Marshal(struct {
		State string `json:"state"`
	}{State: "COMPLETE"})
	require.NoError(t, err)
	completeRevision, err := jobMilestoneKV.Put(context.Background(), jobID, completeStatus)
	require.NoError(t, err)

	// unreachable storage fails the CheckExists in processChunk, causing a nak
	storageURL := "http://localhost:1/" + jobID + "/chunk.mkv"

	_, err = ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, 30*time.Second, test.SilentLogger())
	require.NoError(t, err)

	publishVideoChunk(t, js, pipeline.VideoChunkMessage{
		ChunkRef: pipeline.ChunkRef{JobID: jobID, ChunkIndex: 0}, TotalChunks: 1,
		StorageURL: storageURL, TargetResolution: "480p", Operation: "transcode",
	})

	assertNacked(t, js, "expected message to be nacked")

	entry, err := jobMilestoneKV.Get(context.Background(), jobID)
	require.NoError(t, err)

	var status struct {
		State string `json:"state"`
	}
	require.NoError(t, json.Unmarshal(entry.Value(), &status))
	assert.Equal(t, "COMPLETE", status.State, "processing a chunk after job completion must not regress the milestone")
	assert.Equal(t, completeRevision, entry.Revision(), "AdvanceMilestone must not write at all once the milestone is terminal")
}

func TestConsumeVideoChunk(t *testing.T) {
	t.Run("save failure naks for redelivery", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js, "chunk-processed")
		jobID := "job-nak-save-fail"
		cleanupTestTempFolders(t, jobID+"-0")

		storageURL := test.ServeTestChunks(t, 1) + "/chunk.mkv"
		claimKV := test.SetupKV(t, js, "chunk-claims")
		jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

		_, err := ConsumeVideoChunk("http://localhost:1", nc, js, kv, jobMilestoneKV, claimKV, 30*time.Second, test.SilentLogger())
		require.NoError(t, err)

		publishVideoChunk(t, js, pipeline.VideoChunkMessage{
			ChunkRef: pipeline.ChunkRef{JobID: jobID, ChunkIndex: 0}, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p", Operation: "transcode",
		})

		assertNacked(t, js, "expected message to be nacked")
	})

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

		kv := test.SetupKV(t, js, "chunk-processed")

		jobID := "job-publish-fail"
		cleanupTestTempFolders(t, jobID+"-0")

		storageURL := test.ServeTestChunks(t, 1) + "/chunk.mkv"
		claimKV := test.SetupKV(t, js, "chunk-claims")
		jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

		_, err = ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, 30*time.Second, test.SilentLogger())
		require.NoError(t, err)

		publishVideoChunk(t, js, pipeline.VideoChunkMessage{
			ChunkRef: pipeline.ChunkRef{JobID: jobID, ChunkIndex: 0}, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p", Operation: "transcode",
		})

		assertNacked(t, js, "expected message to be nacked after publish failure")
	})
}

func TestConsumeVideoChunkCleanup(t *testing.T) {
	seedAndConsume := func(t *testing.T, jobID string) (jetstream.JetStream, <-chan struct{}) {
		t.Helper()
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js, "chunk-processed")

		storageURL := test.ServeTestChunks(t, 1) + "/chunk.mkv"
		claimKV := test.SetupKV(t, js, "chunk-claims")
		jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

		_, err := ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, 30*time.Second, test.SilentLogger())
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(_ *nats.Msg) { received <- struct{}{} })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		publishVideoChunk(t, js, pipeline.VideoChunkMessage{
			ChunkRef: pipeline.ChunkRef{JobID: jobID, ChunkIndex: 0}, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p", Operation: "transcode",
		})

		return js, received
	}

	tests := []struct {
		name       string
		jobID      string
		failOnCall int
	}{
		{"removeAll error on processed dir logs warn and returns", "job-cleanup-processed-err", 1},
		{"removeAll error on denoise processed dir logs warn and returns", "job-cleanup-denoise-err", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanupTestTempFolders(t, tc.jobID+"-0")
			t.Cleanup(func() { removeAll = os.RemoveAll })

			calls := 0
			removeAll = func(path string) error {
				if !strings.Contains(path, tc.jobID) {
					return os.RemoveAll(path)
				}
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
			assert.Equal(t, 2, calls)
		})
	}
}

func TestConsumeVideoChunkIdempotency(t *testing.T) {
	t.Run("already processed chunk is acked and skipped", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js, "chunk-processed")

		jobID := "job-idempotency-skip"

		// Pre-seed the KV as if this chunk was already processed.
		_, err := kv.Put(context.Background(), fmt.Sprintf("%s.%d", jobID, 0), []byte("processed"))
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(_ *nats.Msg) { received <- struct{}{} })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })
		claimKV := test.SetupKV(t, js, "chunk-claims")
		jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

		_, err = ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, 30*time.Second, test.SilentLogger())
		require.NoError(t, err)

		publishVideoChunk(t, js, pipeline.VideoChunkMessage{
			ChunkRef: pipeline.ChunkRef{JobID: jobID, ChunkIndex: 0}, TotalChunks: 1,
			StorageURL: "http://storage/fake", TargetResolution: "480p", Operation: "transcode",
		})

		select {
		case <-received:
			t.Fatal("already processed chunk triggered a downstream publish")
		case <-time.After(2 * time.Second):
		}
	})

	t.Run("kv entry is written after successful processing", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js, "chunk-processed")

		jobID := "job-idempotency-write"
		cleanupTestTempFolders(t, jobID+"-0")

		storageURL := test.ServeTestChunks(t, 1) + "/chunk.mkv"
		claimKV := test.SetupKV(t, js, "chunk-claims")
		jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

		_, err := ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, 30*time.Second, test.SilentLogger())
		require.NoError(t, err)

		publishVideoChunk(t, js, pipeline.VideoChunkMessage{
			ChunkRef: pipeline.ChunkRef{JobID: jobID, ChunkIndex: 0}, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p", Operation: "transcode",
		})

		// Wait for processing to complete then verify KV entry exists.
		require.Eventually(t, func() bool {
			_, err := kv.Get(context.Background(), fmt.Sprintf("%s.%d", jobID, 0))
			return err == nil
		}, 30*time.Second, 200*time.Millisecond, "kv entry for processed chunk was never written")
	})

	t.Run("concurrent redelivery does not duplicate a still-in-flight transcode", func(t *testing.T) {
		js, nc := test.SetupNats(t)
		kv := test.SetupKV(t, js, "chunk-processed")
		claimKV := test.SetupKV(t, js, "chunk-claims")
		jobMilestoneKV := test.SetupJobMilestoneKV(t, js)

		jobID := "job-redelivery-race"
		cleanupTestTempFolders(t, jobID+"-0")

		// block real ffmpeg call until test releases it, so first attempt is
		// still "in progress" when AckWait fires and JetStream redelivers to 2nd worker
		var calls int32
		release := make(chan struct{})
		transcodeVideoFn = func(filePath, targetResolution, chunkName string) (string, error) {
			atomic.AddInt32(&calls, 1)
			<-release

			outDir := "/tmp/transcode/temp-processed-" + chunkName
			err := os.MkdirAll(outDir, 0755)
			if err != nil {
				return "", err
			}
			outputPath := filepath.Join(outDir, chunkName+".mp4")
			err = os.WriteFile(outputPath, []byte("fake transcoded output"), 0644)
			if err != nil {
				return "", err
			}

			return outputPath, nil
		}
		t.Cleanup(func() { transcodeVideoFn = transcodeVideo })

		storageURL := test.ServeTestChunks(t, 1) + "/chunk.mkv"

		received := make(chan []byte, 10)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(msg *nats.Msg) { received <- msg.Data })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		const ackWait = 2 * time.Second // short ackWait so test doesnt take forever

		// Two workers sharing same durable consumer to simulate two scaled out chunk-worker instances
		_, err = ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, ackWait, test.SilentLogger())
		require.NoError(t, err)
		_, err = ConsumeVideoChunk(sharedFilerURL, nc, js, kv, jobMilestoneKV, claimKV, ackWait, test.SilentLogger())
		require.NoError(t, err)

		publishVideoChunk(t, js, pipeline.VideoChunkMessage{
			ChunkRef: pipeline.ChunkRef{JobID: jobID, ChunkIndex: 0}, TotalChunks: 1,
			StorageURL: storageURL, TargetResolution: "480p", Operation: "transcode",
		})

		// Ackwait expires and triggers a redelivery to idle worker before releasing blocked transcode(s)
		time.Sleep(ackWait * 3)
		close(release)

		select {
		case <-received:
		case <-time.After(30 * time.Second):
			t.Fatal("timed out waiting for chunk complete message")
		}

		time.Sleep(2 * time.Second) // let duplicate publish arrive if race exists

		assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "chunk should only be transcoded once despite redelivery")
		assert.Len(t, received, 0, "unexpected duplicate chunk-complete publish")
	})
}
