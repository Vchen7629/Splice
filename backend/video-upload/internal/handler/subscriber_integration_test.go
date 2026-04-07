//go:build integration

package handler_test

import (
	"context"
	"testing"
	"time"
	"video-upload/internal/handler"
	"video-upload/internal/test"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

func TestSubscribeErrors(t *testing.T) {
	t.Run("no stream for subject should return error", func(t *testing.T) {
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

		_, _, err = handler.SubscribeJobCompletion(js, test.SilentLogger())

		assert.Error(t, err)
	})
}

func TestSubscribe(t *testing.T) {
	js, _ := test.SetupNats(t)
	tracker, consCtx, err := handler.SubscribeJobCompletion(js, test.SilentLogger())
	require.NoError(t, err)

	t.Run("returns non-nil tracker and consume context", func(t *testing.T) {
		assert.NotNil(t, tracker)
		assert.NotNil(t, consCtx)
	})

	t.Run("consumer is created with the correct config", func(t *testing.T) {
		ctx := context.Background()

		stream, err := js.Stream(ctx, "jobs")
		require.NoError(t, err)

		cons, err := stream.Consumer(ctx, "video-upload-status")
		require.NoError(t, err)

		info, err := cons.Info(ctx)
		require.NoError(t, err)

		assert.Equal(t, "video-upload-status", info.Config.Name)
		assert.Equal(t, "video-upload-status", info.Config.Durable)
		assert.Equal(t, "jobs.complete", info.Config.FilterSubject)
		assert.Equal(t, jetstream.AckExplicitPolicy, info.Config.AckPolicy)
		assert.Equal(t, 50, info.Config.MaxAckPending)
		assert.Equal(t, 3, info.Config.MaxDeliver)
		assert.Equal(t, 30*time.Second, info.Config.AckWait)
	})

	t.Run("valid message adds job ID to tracker", func(t *testing.T) {
		test.PublishJobComplete(t, js, "job-abc")

		assert.Eventually(t, func() bool {
			return tracker.IsDone("job-abc")
		}, 5*time.Second, 50*time.Millisecond)
	})

	t.Run("invalid json does not add any job to tracker", func(t *testing.T) {
		ctx := context.Background()
		_, err = js.Publish(ctx, "jobs.complete", []byte("not valid json"))
		require.NoError(t, err)

		select {
		case <-time.After(2 * time.Second):
			assert.False(t, tracker.IsDone("not valid json"))
		}
	})
}
