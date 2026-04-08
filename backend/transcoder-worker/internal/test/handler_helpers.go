package test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"
	"transcoder-worker/internal/service"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func SilentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func PublishVideoChunk(t *testing.T, js jetstream.JetStream, msg service.VideoChunkMessage) {
	t.Helper()
	payload, err := json.Marshal(msg)
	require.NoError(t, err)
	_, err = js.Publish(context.Background(), "jobs.video.chunks", payload)
	require.NoError(t, err)
}

// assertNacked polls until the named consumer has a pending ack, confirming the message was nacked.
func AssertNacked(t *testing.T, js jetstream.JetStream, msg string) {
	t.Helper()
	ctx := context.Background()
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
	}, 30*time.Second, 200*time.Millisecond, msg)
}
