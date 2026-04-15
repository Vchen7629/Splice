//go:build integration

package handler

import (
	"context"
	"os"
	"testing"
	"video-recombiner/internal/test"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// connects to existing job-status bucket
func TestConnectJobStatusKV(t *testing.T) {
	t.Run("connects to existing job-status bucket", func(t *testing.T) {
		js, _ := test.SetupNats(t)

		_, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
			Bucket: "job-status",
		})
		require.NoError(t, err)

		kv := ConnectJobStatusKV(js, test.SilentLogger())

		assert.NotNil(t, kv)
	})

	t.Run("exits when job-status bucket does not exist", func(t *testing.T) {
		js, _ := test.SetupNats(t)

		code := -1
		osExit = func(c int) { code = c }
		t.Cleanup(func() { osExit = os.Exit })

		ConnectJobStatusKV(js, test.SilentLogger())

		assert.Equal(t, 1, code)
	})
}
