//go:build integration

package jetstream

import (
	"context"
	"os"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/test"
)

// connects to existing KV bucket
func TestConnectKV(t *testing.T) {
	const bucketName = "job-status"

	t.Run("connects to existing job-status bucket", func(t *testing.T) {
		js, _ := test.SetupNats(t)

		_, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
			Bucket: bucketName,
		})
		require.NoError(t, err)

		kv := ConnectKV(js, bucketName, test.SilentLogger())

		assert.NotNil(t, kv)
	})

	t.Run("exits when job-status bucket does not exist", func(t *testing.T) {
		js, _ := test.SetupNats(t)

		code := -1
		osExit = func(c int) { code = c }
		t.Cleanup(func() { osExit = os.Exit })

		ConnectKV(js, bucketName, test.SilentLogger())

		assert.Equal(t, 1, code)
	})
}
