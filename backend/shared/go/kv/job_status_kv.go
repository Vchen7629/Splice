package kv

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

var osExit = os.Exit

// connect to existing job status kv to publishing the processing stage update msgs
func ConnectJobStatus(js jetstream.JetStream, logger *slog.Logger) jetstream.KeyValue {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	kv, err := js.KeyValue(ctx, "job-status")
	if err != nil {
		logger.Error("failed to connect to job-status kv bucket", "err", err)
		osExit(1)
	}

	return kv
}

// publish a msg to the job status KV with the current processing stage and PROCESSING msg
func UpdateJobStatus(
	jobStatusKV jetstream.KeyValue, jobStage, jobID string, logger *slog.Logger,
) error {
	status, err := json.Marshal(struct {
		State string `json:"state"`
		Stage string `json:"stage"`
	}{State: "PROCESSING", Stage: jobStage})
	if err != nil {
		logger.Error("error marshalling status text", "err", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = jobStatusKV.Put(ctx, jobID, status)
	if err != nil {
		logger.Error("failed to write job status to jobStatus kv", "job_id", jobID, "err", err)
		return err
	}

	return nil
}
