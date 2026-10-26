package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/msgbus"
)

// reads a job's milestone entry for the jobID and returns the revision, JobStatus, and err (if applicable)
func GetMilestone(kv jetstream.KeyValue, jobID string) (uint64, JobStatus, error) {
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

// ranks pipeline stages so AdvanceMilestone can differentiate forward write from stale one
var milestoneStageOrder = map[string]int{
	"upload":           0,
	"chunk-worker":     1,
	"format-converter": 1,
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
		revision, jobStatus, err := GetMilestone(kv, jobID)
		if err != nil {
			return fmt.Errorf("failed: %w", err)
		}
		if revision == 0 {
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

// terminate the nats msg if the job is cancelled and return true
func TerminateIfCancelled(
	jobMilestoneKV jetstream.KeyValue, msg jetstream.Msg, jobID string, logger *slog.Logger,
) (shouldCancel, stopJob bool) {
	_, milestoneStatus, err := GetMilestone(jobMilestoneKV, jobID)
	if err != nil {
		logger.Error("failed to fetch milestoneStatus from KV", "job_id", jobID, "err", err)
		msgbus.NakWithErrHandling(logger, msg)
		return false, true
	}
	if milestoneStatus.State != StateCancelled {
		return false, false
	}

	err = msg.Term()
	if err != nil {
		logger.Error("failed to terminate the cancelled jetstream msg", "job_id", jobID, "err", err)
	}
	return true, true
}
