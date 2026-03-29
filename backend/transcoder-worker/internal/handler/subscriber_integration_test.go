//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	"transcoder-worker/internal/handler"
	"transcoder-worker/internal/service"
	"transcoder-worker/internal/test"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

const testVideoPath = "../test/test_video.mp4"

func TestConsumeVideoChunkErrors(t *testing.T) {
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

		_, err = handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

		assert.Error(t, err)
	})
}

func TestConsumeVideoChunkSuccess(t *testing.T) {
	t.Run("returns non-nil consume context", func(t *testing.T) {
		js, _ := test.SetupNats(t)

		consCtx, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

		require.NoError(t, err)
		assert.NotNil(t, consCtx)
	})
}

func TestConsumeVideoChunkConsumerConfig(t *testing.T) {
	t.Run("consumer is created with the correct config", func(t *testing.T) {
		ctx := context.Background()
		js, _ := test.SetupNats(t)

		_, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())
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
}

func TestConsumeVideoChunkMessageHandling(t *testing.T) {
	t.Run("invalid JSON does not publish downstream", func(t *testing.T) {
		js, nc := test.SetupNats(t)

		_, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())
		require.NoError(t, err)

		received := make(chan struct{}, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(_ *nats.Msg) {
			received <- struct{}{}
		})
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

	t.Run("valid transcode publishes chunk complete message and acks", func(t *testing.T) {
		js, nc := test.SetupNats(t)

		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(m *nats.Msg) {
			received <- m.Data
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		_, err = handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())
		require.NoError(t, err)

		payload, err := json.Marshal(service.VideoChunkMessage{
			JobID:            "job-1",
			ChunkIndex:       2,
			TotalChunks:      5,
			StoragePath:      testVideoPath,
			TargetResolution: "480p",
		})
		require.NoError(t, err)

		_, err = js.Publish(context.Background(), "jobs.video.chunks", payload)
		require.NoError(t, err)

		select {
		case data := <-received:
			var msg service.ChunkCompleteMessage
			require.NoError(t, json.Unmarshal(data, &msg))
			assert.Equal(t, "job-1", msg.JobID)
			assert.Equal(t, 2, msg.ChunkIndex)
			assert.Equal(t, 5, msg.TotalChunks)
			assert.NotEmpty(t, msg.OutputPath)
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for chunk complete message")
		}
	})
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

		// Stream only covers input subject — js.Publish to jobs.chunks.complete will error
		_, err = js.CreateStream(ctx, jetstream.StreamConfig{
			Name:     "jobs",
			Subjects: []string{"jobs.video.chunks"},
		})
		require.NoError(t, err)

		_, err = handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())
		require.NoError(t, err)

		payload, err := json.Marshal(service.VideoChunkMessage{
			JobID:            "job-1",
			ChunkIndex:       0,
			TotalChunks:      1,
			StoragePath:      testVideoPath,
			TargetResolution: "480p",
		})
		require.NoError(t, err)

		_, err = js.Publish(ctx, "jobs.video.chunks", payload)
		require.NoError(t, err)

		// NumAckPending > 0 means the message was delivered to the consumer but not acked.
		// It stays > 0 for the 30s AckWait window, confirming nak was called (not ack).
		require.Eventually(t, func() bool {
			stream, err := js.Stream(ctx, "jobs")
			if err != nil {
				return false
			}
			cons, err := stream.Consumer(ctx, "transcoder-worker")
			if err != nil {
				return false
			}
			info, err := cons.Info(ctx)
			if err != nil {
				return false
			}
			return info.NumAckPending > 0
		}, 15*time.Second, 200*time.Millisecond, "expected message to be nacked and pending redelivery")
	})
}
