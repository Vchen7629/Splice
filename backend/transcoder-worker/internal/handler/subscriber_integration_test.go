//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	"transcoder-worker/internal/handler"
	"transcoder-worker/internal/test"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

func TestNoStream_ReturnsError(t *testing.T) {
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
}

func TestReturnsConsumeContext(t *testing.T) {
	js, _ := test.SetupNats(t)

	consCtx, err := handler.ConsumeVideoChunk(js, test.SilentLogger(), t.TempDir())

	require.NoError(t, err)
	assert.NotNil(t, consCtx)
}

func TestCreatesConsumerWithCorrectConfig(t *testing.T) {
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
}

func TestInvalidJSON_DoesNotPublishDownstream(t *testing.T) {
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
}
