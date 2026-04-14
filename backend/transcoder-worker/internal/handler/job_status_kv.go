package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// connect to existing job status kv to publishing the processing stage update msgs
func ConnectJobStatusKV(js jetstream.JetStream, logger *slog.Logger) jetstream.KeyValue {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := js.KeyValue(ctx, "job-status")
	if err != nil {
		logger.Error("failed to create recombine-chunk-recieved kv bucket", "err", err)
		osExit(1)
	}

	return kv
}

func UpdateJobStatusKV(jobStatusKV jetstream.KeyValue, JobID string, logger *slog.Logger) error {
	status, err := json.Marshal(struct {
		State string `json:"state"`
		Stage string `json:"stage"`
	}{State: "PROCESSING", Stage: "transcoder"})
	if err != nil {
		logger.Error("error marshalling status text", "err", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = jobStatusKV.Put(ctx, JobID, status)
	if err != nil {
		logger.Error("failed to write job status to jobStatus kv", "job_id", JobID, "err", err)
		return err
	}

	return nil
}