package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

var osExit = os.Exit

// Create the Msg Processed KV store for idempotency
func CreateMsgProcessedKV(js jetstream.JetStream, logger *slog.Logger) jetstream.KeyValue {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "transcode-chunk-job-processed",
		Description: "tracks already completed video chunk for the jobID is already processed for idempotency",
		TTL:         3 * time.Hour,
	})
	if err != nil {
		logger.Error("failed to create transcode-chunk-job-processed kv bucket", "err", err)
		osExit(1)
	}

	return kv
}

// check if a jobID chunk already is processed, returns a bool based on if it exists in the KV
func CheckChunkProcessed(kv jetstream.KeyValue, jobID string, chunkIndex int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := kv.Get(ctx, fmt.Sprintf("%s.%d", jobID, chunkIndex))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed: %w", err)
	}

	return true, nil
}

// add a completed job chunk to the KV for idempotency
func AddChunkProcessed(kv jetstream.KeyValue, jobID string, chunkIndex int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := kv.Put(ctx, fmt.Sprintf("%s.%d", jobID, chunkIndex), []byte("processed"))
	if err != nil {
		return fmt.Errorf("failed: %w", err)
	}

	return nil
}
