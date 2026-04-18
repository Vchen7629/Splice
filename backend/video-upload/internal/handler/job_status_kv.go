package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/nats-io/nats.go/jetstream"
)

var osExit = os.Exit

func updateJobStatusKV(ctx context.Context, jobID string, kv jetstream.KeyValue, logger *slog.Logger) error {
	status, err := json.Marshal(struct {
		State string `json:"state"`
		Stage string `json:"stage"`
	}{State: "PROCESSING", Stage: "upload"})
	if err != nil {
		logger.Error("error marshalling PROCESSING:upload text", "err", err)
		return err
	}

	_, err = kv.Put(ctx, jobID, status)
	if err != nil {
		logger.Error("failed to write job status to jetstream kv", "job_id", jobID, "err", err)
		return err
	}

	return nil
}
