//go:build integration

package handler_test

import (
	"context"
	"shared/handler"
	"shared/test"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorrectConfig(t *testing.T) {
	ctx := context.Background()
	js, _ := test.SetupNats(t)

	cons, err := handler.CreateDurableConsumer(js, "jobs.chunks.complete", "video-recombiner")
	require.NoError(t, err)

	info, err := cons.Info(ctx)
	require.NoError(t, err)

	assert.Equal(t, "video-recombiner", info.Config.Name)
	assert.Equal(t, "video-recombiner", info.Config.Durable)
	assert.Equal(t, "jobs.chunks.complete", info.Config.FilterSubject)
	assert.Equal(t, jetstream.AckExplicitPolicy, info.Config.AckPolicy)
	assert.Equal(t, 10, info.Config.MaxAckPending)
	assert.Equal(t, 3, info.Config.MaxDeliver)
	assert.Equal(t, 30*time.Second, info.Config.AckWait)
}
