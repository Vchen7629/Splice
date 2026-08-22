package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/nats-io/nats.go/jetstream"
)

var osExit = os.Exit

type KVHandler struct {
	logger *slog.Logger
	kv     jetstream.KeyValue
}

func (h *KVHandler) getJobStatusKV(ctx context.Context, jobID string) (jetstream.KeyValueEntry, int, error) {
	entry, err := h.kv.Get(ctx, jobID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, http.StatusNotFound, errors.New("job not found")
		}
		h.logger.Error("failed to get job status from kv", "job_id", jobID, "err", err)
		return nil, http.StatusInternalServerError, errors.New("failed to get job status")
	}

	return entry, http.StatusOK, nil
}

func (h *KVHandler) updateJobStatusKV(ctx context.Context, JobID string, status JobStatus) error {
	data, err := json.Marshal(status)
	if err != nil {
		h.logger.Error("error marshalling status", "err", err)
		return err
	}

	_, err = h.kv.Put(ctx, JobID, data)
	if err != nil {
		h.logger.Error("failed to write job status to jobStatus kv", "job_id", JobID, "err", err)
		return err
	}

	return nil
}
