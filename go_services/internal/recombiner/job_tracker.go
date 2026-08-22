package recombiner

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
)

const chunkKeyPrefix = "chunk."

// record a completed chunk for a job from nats msgs, returns ready=true and a map of all chunk paths when all video
// chunks for the job has been recieved so the subscriber can trigger combiner.go and pass in the mapping to combine all.
// chunk records are left in KV once read so retry of a triggering chunk after a failed combine can still see every chunk's
// storage URL. cleanup is handled eventually by bucket TTL
func Add(kv jetstream.KeyValue, payload handler.ChunkCompleteMessage, logger *slog.Logger) (ready bool, chunks map[int]string, err error) {
	key := fmt.Sprintf("%s%s.%d", chunkKeyPrefix, payload.JobID, payload.ChunkIndex)
	err = sJetstream.PutKeyKV(kv, key, []byte(payload.StorageURL))
	if err != nil {
		return false, nil, err
	}

	chunks, err = listJobChunksKV(kv, payload.JobID, logger)
	if err != nil {
		return false, nil, err
	}
	if len(chunks) < payload.TotalChunks {
		return false, nil, nil
	}

	return true, chunks, nil
}

// returns every chunk recorded so far for jobID as chunkIndex -> storageURL
func listJobChunksKV(kv jetstream.KeyValue, jobID string, logger *slog.Logger) (map[int]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	filter := fmt.Sprintf("%s%s.*", chunkKeyPrefix, jobID)
	lister, err := kv.ListKeysFiltered(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed: %w", err)
	}
	defer func() {
		err := lister.Stop()
		if err != nil {
			logger.Warn("failed to stop lister at the end")
		}
	}()

	keyPrefix := chunkKeyPrefix + jobID + "."
	chunks := make(map[int]string)
	for key := range lister.Keys() {
		idx, err := strconv.Atoi(strings.TrimPrefix(key, keyPrefix))
		if err != nil {
			return nil, fmt.Errorf("failed to parse chunk index from key %q: %w", key, err)
		}

		entry, err := kv.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed: %w", err)
		}
		chunks[idx] = string(entry.Value())
	}

	return chunks, nil
}
