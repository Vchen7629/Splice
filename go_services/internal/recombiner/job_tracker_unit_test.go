//go:build unit

package recombiner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	shandler "splice.com/go_services/internal/shared/handler"
	"splice.com/go_services/internal/shared/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chunkMsg(idx, total int) shandler.ChunkCompleteMessage {
	return shandler.ChunkCompleteMessage{
		ChunkRef:    shandler.ChunkRef{JobID: "job-1", ChunkIndex: idx},
		TotalChunks: total,
		StorageURL:  fmt.Sprintf("/tmp/chunk-%d.mp4", idx),
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name       string
		total      int
		deliveries []int // chunk indexes delivered in order; assertions apply to the last Add
		wantReady  bool
		wantChunks map[int]string
	}{
		{
			name:       "not ready until all chunks recorded",
			total:      2,
			deliveries: []int{0},
		},
		{
			name:       "redelivered chunk is not double counted",
			total:      2,
			deliveries: []int{0, 0},
		},
		{
			name:       "ready returns the full chunk map",
			total:      2,
			deliveries: []int{0, 1},
			wantReady:  true,
			wantChunks: map[int]string{0: "/tmp/chunk-0.mp4", 1: "/tmp/chunk-1.mp4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := &test.MockKV{}

			var (
				ready  bool
				chunks map[int]string
				err    error
			)
			for _, idx := range tt.deliveries {
				ready, chunks, err = Add(kv, chunkMsg(idx, tt.total), test.SilentLogger())
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantReady, ready)
			assert.Equal(t, tt.wantChunks, chunks)
		})
	}

	t.Run("does not read chunk values before all chunks are in", func(t *testing.T) {
		// Get is stubbed to fail, so any value read before the last chunk would surface as an error
		kv := &test.MockKV{GetErr: errors.New("get must not be called")}

		for idx := range 2 {
			ready, chunks, err := Add(kv, chunkMsg(idx, 3), test.SilentLogger())
			require.NoError(t, err)
			assert.False(t, ready)
			assert.Nil(t, chunks)
		}
	})
}

func TestFetchJobChunksKV(t *testing.T) {
	t.Run("errors on a non numeric chunk index", func(t *testing.T) {
		got, err := fetchJobChunksKV(context.Background(), &test.MockKV{}, "job-1", []string{"chunk.job-1.abc"})

		require.ErrorContains(t, err, "failed to parse chunk index")
		assert.Nil(t, got)
	})
}
