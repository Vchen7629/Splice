//go:build integration

package kv

import (
	"context"
	"os"
	"shared/test"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// connects to existing job-status bucket
func TestConnectJobStatus(t *testing.T) {
	t.Run("connects to existing job-status bucket", func(t *testing.T) {
		js, _ := test.SetupNats(t)

		_, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
			Bucket: "job-status",
		})
		require.NoError(t, err)

		kv := ConnectJobStatus(js, test.SilentLogger())

		assert.NotNil(t, kv)
	})

	t.Run("exits when job-status bucket does not exist", func(t *testing.T) {
		js, _ := test.SetupNats(t)

		code := -1
		osExit = func(c int) { code = c }
		t.Cleanup(func() { osExit = os.Exit })

		ConnectJobStatus(js, test.SilentLogger())

		assert.Equal(t, 1, code)
	})
}
