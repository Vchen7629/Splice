//go:build unit

package transcoder

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"splice.com/go_services/internal/shared/test"
)

func TestJobProgressPct(t *testing.T) {
	ctx := context.Background()

	t.Run("No chunks processed yet returns 0 percent", func(t *testing.T) {
		kv := &test.MockKV{}

		pct, err := jobProgressPct(ctx, kv, "job-1", 4, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, 0, pct)
	})

	t.Run("all chunks processed returns 100 percent", func(t *testing.T) {
		kv := &test.MockKV{}
		for i := range 4 {
			_, err := kv.Put(ctx, fmt.Sprintf("job-1.%d", i), []byte("processed"))
			require.NoError(t, err)
		}

		pct, err := jobProgressPct(ctx, kv, "job-1", 4, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, 100, pct)
	})

	t.Run("some chunks processed returns partial percent", func(t *testing.T) {
		kv := &test.MockKV{}
		for _, i := range []int{0, 1} {
			_, err := kv.Put(ctx, fmt.Sprintf("job-1.%d", i), []byte("processed"))
			require.NoError(t, err)
		}

		pct, err := jobProgressPct(ctx, kv, "job-1", 4, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, 50, pct)
	})

	t.Run("a different job's keys are not counted", func(t *testing.T) {
		kv := &test.MockKV{}
		for i := range 4 {
			_, err := kv.Put(ctx, fmt.Sprintf("job-10.%d", i), []byte("processed"))
			require.NoError(t, err)
		}

		pct, err := jobProgressPct(ctx, kv, "job-1", 4, test.SilentLogger())

		require.NoError(t, err)
		assert.Equal(t, 0, pct, "job-10's chunks must not count toward job-1's progress")
	})
}
