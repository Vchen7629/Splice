//go:build integration

package test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

// Starts a NATS container with JetStream enabled and creates a jobs stream.
// Returns js, nc, and a cleanup function
func StartNats() (jetstream.JetStream, *nats.Conn, func()) {
	ctx := context.Background()

	container, err := natstc.Run(ctx, "nats:2.10-alpine")
	if err != nil {
		panic("failed to start NATS container: " + err.Error())
	}

	url, err := container.ConnectionString(ctx)
	if err != nil {
		panic("failed to get NATS connection string: " + err.Error())
	}

	nc, err := nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(200*time.Millisecond),
	)
	if err != nil {
		panic("failed to connect to NATS: " + err.Error())
	}

	js, err := jetstream.New(nc)
	if err != nil {
		panic("failed to create JetStream context: " + err.Error())
	}

	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "jobs",
		Subjects: []string{"jobs.>"},
	})
	if err != nil {
		panic("failed to create jobs stream: " + err.Error())
	}

	return js, nc, func() {
		nc.Close()
		_ = container.Terminate(ctx)
	}
}

// Creates the job-status KV bucket on the given JetStream instance.
func CreateKV(js jetstream.JetStream) jetstream.KeyValue {
	kv, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket: "job-status",
	})
	if err != nil {
		panic("failed to create job-status KV bucket: " + err.Error())
	}
	return kv
}

// Publishes payload to subject and returns the assigned stream sequence number.
func SeedStreamMessage(t *testing.T, js jetstream.JetStream, subject string, payload []byte) uint64 {
	t.Helper()
	ack, err := js.Publish(context.Background(), subject, payload)
	require.NoError(t, err)
	return ack.Sequence
}

// Mirrors the max-delivery advisory shape that JetStream publishes.
type advisoryPayload struct {
	Stream    string `json:"stream"`
	Consumer  string `json:"consumer"`
	StreamSeq uint64 `json:"stream_seq"`
}

// Publishes a fake max-delivery advisory to core NATS so the handler callback fires immediately.
func PublishAdvisory(t *testing.T, nc *nats.Conn, stream, consumer string, seq uint64) {
	t.Helper()
	subject := "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES." + stream
	data, err := json.Marshal(advisoryPayload{Stream: stream, Consumer: consumer, StreamSeq: seq})
	require.NoError(t, err)
	require.NoError(t, nc.Publish(subject, data))
}
