//go:build integration

package test

import (
	"context"
	"encoding/json"
	"testing"
	"video-upload/internal/service"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

// fixture for setting up nats container for testing
func SetupNats(t *testing.T) (jetstream.JetStream, *nats.Conn) {
	t.Helper()
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

	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "jobs",
		Subjects: []string{"jobs.>"},
	})
	require.NoError(t, err)

	return js, nc
}

// PublishJobComplete publishes a JobCompleteMessage to jobs.complete, simulating
// the downstream video processor signalling that a job has finished.
func PublishJobComplete(t *testing.T, js jetstream.JetStream, jobID string) {
	t.Helper()
	payload, err := json.Marshal(service.JobCompleteMessage{JobID: jobID})
	require.NoError(t, err)
	_, err = js.Publish(context.Background(), "jobs.complete", payload)
	require.NoError(t, err)
}
