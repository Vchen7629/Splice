package transcoder

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
)

// returns percent of a job's chunks that has been marked processed in processedKV
// read fresh from KV each call so it's monotonic with multiple horizontally scaled workers
func jobProgressPct(ctx context.Context, processedKV jetstream.KeyValue, jobID string, totalChunks int, logger *slog.Logger) (int, error) {
	if totalChunks <= 0 {
		return 0, nil
	}

	prefix := jobID + "."
	lister, err := processedKV.ListKeysFiltered(ctx, prefix+"*")
	if err != nil {
		return 0, fmt.Errorf("failed to list processed chunk keys: %w", err)
	}
	defer func() {
		stopErr := lister.Stop()
		if stopErr != nil {
			logger.Warn("failed to stop lister at the end")
		}
	}()

	completed := 0
	for key := range lister.Keys() {
		if strings.HasPrefix(key, prefix) {
			completed++
		}
	}

	return completed * 100 / totalChunks, nil
}
