//go:build integration

package test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
	"github.com/testcontainers/testcontainers-go/wait"
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

	nc, err := nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(200*time.Millisecond),
	)
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

// starts a plain NATS container without JetStream enabled and returns the connection.
func SetupNatsNoJetStream(t *testing.T) *nats.Conn {
	t.Helper()
	ctx := context.Background()

	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: tc.ContainerRequest{
			Image:        "nats:2.10-alpine",
			ExposedPorts: []string{"4222/tcp"},
			WaitingFor:   wait.ForLog("Server is ready"),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "4222")
	require.NoError(t, err)

	nc, err := nats.Connect("nats://" + host + ":" + port.Port())
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	return nc
}

func SetupKV(t *testing.T, js jetstream.JetStream) jetstream.KeyValue {
	t.Helper()
	kv, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket: "chunk-processed",
	})
	require.NoError(t, err)
	return kv
}
