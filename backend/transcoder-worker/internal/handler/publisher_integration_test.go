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

func TestPublishChunkCompleteI(t *testing.T) {
	t.Run("publishes correct payload to downstream subject", func(t *testing.T) {
		js, nc := test.SetupNats(t)

		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.chunks.complete", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		msg := service.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  2,
			TotalChunks: 1,
			StorageURL:  "/output/chunk-2.mp4",
		}

		fn := handler.PublishChunkComplete(js)
		err = fn(msg)
		require.NoError(t, err)

		select {
		case data := <-received:
			var got service.ChunkCompleteMessage
			require.NoError(t, json.Unmarshal(data, &got))
			assert.Equal(t, msg, got)
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("no stream returns error", func(t *testing.T) {
		ctx := context.Background()

		container, err := natstc.Run(ctx, "nats:2.10-alpine")
		require.NoError(t, err)
		t.Cleanup(func() { _ = container.Terminate(ctx) })

		url, err := container.ConnectionString(ctx)
		require.NoError(t, err)

		nc, err := nats.Connect(url,
			nats.RetryOnFailedConnect(true),
			nats.MaxReconnects(10),
			nats.ReconnectWait(200*time.Millisecond),
		)
		require.NoError(t, err)
		t.Cleanup(nc.Close)

		// JetStream with no stream configured for the subject
		js, err := jetstream.New(nc)
		require.NoError(t, err)

		fn := handler.PublishChunkComplete(js)
		err = fn(service.ChunkCompleteMessage{
			JobID:       "job-1",
			ChunkIndex:  0,
			TotalChunks: 1,
			StorageURL:  "/output/chunk-0.mp4",
		})

		assert.Error(t, err)
	})
}
