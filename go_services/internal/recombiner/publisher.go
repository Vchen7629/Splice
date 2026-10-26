package recombiner

import (
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/msgbus"
	"splice.com/go_services/internal/shared/pipeline"
)

// publishes a jetstream msg to mark the job as complete
func publishJetstreamCompleteMsg(
	js jetstream.JetStream, msgRecievedKV jetstream.KeyValue, payload pipeline.ChunkCompleteMessage, logger *slog.Logger,
) error {
	const pubSubject = "jobs.complete"
	err := msgbus.Publish(js, pipeline.JobCompleteMessage{JobID: payload.JobID}, pubSubject)
	if err != nil {
		logger.Error("failed to pub msg for video processing complete", "job_id", payload.JobID, "err", err)
		return err
	}

	if err := msgbus.MarkProcessed(msgRecievedKV, payload.ChunkKVKey(), logger); err != nil {
		return err
	}

	logger.Debug("job complete", "job_id", payload.JobID)

	return nil
}
