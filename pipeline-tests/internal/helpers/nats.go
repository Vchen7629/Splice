//go:build integration

package helpers

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	natstc "github.com/testcontainers/testcontainers-go/modules/nats"
)

func StartNats(t *testing.T) (string, jetstream.JetStream) {
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

	return url, js
}

func MergeEnv(overrides map[string]string) []string {
	keys := make(map[string]bool, len(overrides))
	for k := range overrides {
		keys[k] = true
	}
	filtered := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !keys[strings.SplitN(e, "=", 2)[0]] {
			filtered = append(filtered, e)
		}
	}
	for k, v := range overrides {
		filtered = append(filtered, k+"="+v)
	}
	return filtered
}
