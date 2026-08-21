package kv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// creates the chunkClaimKV used to prevent duplicate chunk redelivery from reprocessing the chunk
func CreateChunkClaimKV(bucketName string, js jetstream.JetStream, ttl time.Duration, logger *slog.Logger) jetstream.KeyValue {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: bucketName,
		TTL:    ttl,
	})
	if err != nil {
		logger.Error("failed to create chunk-claim kv bucket", "err", err)
		osExit(1)
		return nil
	}

	return kv
}

// ClaimChunk atomically claims a jobID/chunkIndex pair for processing. Returns claimed=false
// (no error) if another worker already holds the claim, so the caller can tell "I own this"
// apart from "someone else does" apart from a real KV failure.
func ClaimChunk(claimKV jetstream.KeyValue, jobID string, chunkIndex int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := claimKV.Create(ctx, fmt.Sprintf("%s.%d", jobID, chunkIndex), []byte("in-progress"))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return false, nil
		}
		return false, fmt.Errorf("failed: %w", err)
	}

	return true, nil
}

// ReleaseChunkClaim releases a chunk claim so a legitimate retry can reclaim it.
func ReleaseChunkClaim(claimKV jetstream.KeyValue, jobID string, chunkIndex int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := claimKV.Delete(ctx, fmt.Sprintf("%s.%d", jobID, chunkIndex))
	if err != nil {
		return fmt.Errorf("failed: %w", err)
	}

	return nil
}
