package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// check if a jobID chunk already is processed, returns a bool based on if it exists in the KV
func CheckChunkProcessed(kv jetstream.KeyValue, jobID string, chunkIndex int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := kv.Get(ctx, fmt.Sprintf("%s.%d", jobID, chunkIndex))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check chunk processed: %w", err)
	}

	return true, nil
}

// add a completed job chunk to the KV for idempotency
func AddChunkProcessed(kv jetstream.KeyValue, jobID string, chunkIndex int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := kv.Put(ctx, fmt.Sprintf("%s.%d", jobID, chunkIndex), []byte("processed"))
	if err != nil {
		return fmt.Errorf("failed to mark job chunk as processed", "err", err)
	}

	return nil
}
