package jetstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

var osExit = os.Exit

// create the jetstream KV bucket for use in application
func CreateKV(
	bucketName string, js jetstream.JetStream, ttl time.Duration, logger *slog.Logger,
) jetstream.KeyValue {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: bucketName,
		TTL:    ttl,
	})
	if err != nil {
		logger.Error("failed to create kv bucket", "bucketName", bucketName, "err", err)
		osExit(1)
		return nil
	}

	return kv
}

// connect to existing kv
func ConnectKV(js jetstream.JetStream, bucketName string, logger *slog.Logger) jetstream.KeyValue {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := js.KeyValue(ctx, bucketName)
	if err != nil {
		logger.Error("failed to connect to kv bucket", "bucketName", bucketName, "err", err)
		osExit(1)
	}

	return kv
}

// check if a key exists in the KV
func CheckKeyExist(kv jetstream.KeyValue, key string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed: %w", err)
	}

	return true, nil
}

// puts a new key value pair into the jetstream kv. Note the context is hardc
func PutKeyKV(kv jetstream.KeyValue, key string, value []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := kv.Put(ctx, key, value)
	if err != nil {
		return fmt.Errorf("failed: %w", err)
	}

	return nil
}

// reads a job's milestone entry for the jobID and returns the revision, JobStatus, and err (if applicable)
func GetMilestoneKV(kv jetstream.KeyValue, jobID string) (uint64, JobStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	entry, err := kv.Get(ctx, jobID)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return 0, JobStatus{}, nil
	}
	if err != nil {
		return 0, JobStatus{}, fmt.Errorf("failed to fetch from kv: %w", err)
	}

	var current JobStatus
	err = json.Unmarshal(entry.Value(), &current)
	if err != nil {
		return 0, JobStatus{}, fmt.Errorf("failed to unmarshal json: %w", err)
	}

	return entry.Revision(), current, nil
}

// For job mileston kv

// ranks pipeline stages so AdvanceMilestone can differentiate forward write from stale one
var milestoneStageOrder = map[string]int{
	"upload":           0,
	"transcoder":       1,
	"video-recombiner": 2,
}

// writes a job's milestone entry only if newStage is not behind currently stored stage and
// job isnt already in terminal state.
func AdvanceMilestone(kv jetstream.KeyValue, jobID string, newStatus JobStatus) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	newValue, err := json.Marshal(newStatus)
	if err != nil {
		return fmt.Errorf("failed: %w", err)
	}

	for {
		revision, jobStatus, err := GetMilestoneKV(kv, jobID)
		if err != nil {
			return fmt.Errorf("failed: %w", err)
		}
		if jobStatus.State == "" {
			_, err = kv.Create(ctx, jobID, newValue)
			if errors.Is(err, jetstream.ErrKeyExists) {
				continue // lost create race, reread and compare against winner
			}
			if err != nil {
				return fmt.Errorf("failed: %w", err)
			}
			return nil
		}

		if IsStaleTransition(jobStatus, newStatus) {
			return nil
		}

		_, err = kv.Update(ctx, jobID, newValue, revision)
		if errors.Is(err, jetstream.ErrKeyExists) {
			continue // revision change concurrently, reread and compare again
		}
		if err != nil {
			return fmt.Errorf("failed: %w", err)
		}
		return nil
	}
}

// For video/file chunks

// atomically claims jobID/chunkIndex via claimKV and if claim succeeds runs fn
// fn should return completed=true when work is durable persisted and false otherwise
// so legitimate retry can reclaim the chunk. On success claim is left in place to
// expire via claimKV TTL rather than released immediately
func ClaimAndRun(
	claimKV jetstream.KeyValue, jobID string, chunkIndex int, logger *slog.Logger, fn func() (completed bool),
) (bool, error) {
	claimed, err := claimChunk(claimKV, jobID, chunkIndex)
	if err != nil {
		return false, err
	}

	if !claimed {
		return false, nil
	}

	// track whether processing finished successfully so defer release chunk fires on failure path
	// this lets a legit retry reclaim the chunk. On success the claim is left to expire via claimKV TTL
	completed := false
	defer func() {
		if !completed {
			relErr := releaseChunkClaim(claimKV, jobID, chunkIndex)
			if relErr != nil {
				logger.Error("failed to release chunk claim", "job_id", jobID, "chunk_index", chunkIndex, "err", relErr)
			}
		}
	}()

	completed = fn()

	return true, nil
}

// ClaimChunk atomically claims a jobID/chunkIndex pair for processing. Returns claimed=false
// (no error) if another worker already holds the claim, so the caller can tell "I own this"
// apart from "someone else does" apart from a real KV failure.
func claimChunk(claimKV jetstream.KeyValue, jobID string, chunkIndex int) (bool, error) {
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
func releaseChunkClaim(claimKV jetstream.KeyValue, jobID string, chunkIndex int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := claimKV.Delete(ctx, fmt.Sprintf("%s.%d", jobID, chunkIndex))
	if err != nil {
		return fmt.Errorf("failed: %w", err)
	}

	return nil
}
