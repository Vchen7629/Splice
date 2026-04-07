//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	"video-upload/internal/handler"
	"video-upload/internal/service"
	"video-upload/internal/test"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

func TestPublishVideoMetadata(t *testing.T) {
	t.Run("returns error when no stream exists for subject", func(t *testing.T) {
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

		err = handler.PublishVideoMetadata(js, service.SceneSplitMessage{JobID: "job-1"})

		assert.Error(t, err)
	})

	t.Run("publishes correct payload to NATS", func(t *testing.T) {
		js, nc := test.SetupNats(t)

		received := make(chan []byte, 1)
		sub, err := nc.Subscribe("jobs.video.scene-split", func(msg *nats.Msg) {
			received <- msg.Data
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })

		msg := service.SceneSplitMessage{JobID: "job-1"}
		err = handler.PublishVideoMetadata(js, msg)
		require.NoError(t, err)

		select {
		case data := <-received:
			var got service.SceneSplitMessage
			require.NoError(t, json.Unmarshal(data, &got))
			assert.Equal(t, msg, got)
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for message")
		}
	})
}
