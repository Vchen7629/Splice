package converter

import (
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/msgbus"
	"splice.com/go_services/internal/shared/pipeline"
)

const pubSubject = "jobs.complete"

// publishes jobs.complete so gateway's ListenJobComplete marks the job's milestone COMPLETE
// then marks the job processed in processedKV so a redelivered msg isn't reprocessed
func publishJetstreamProcessedMsg(js jetstream.JetStream, processedKV jetstream.KeyValue, jobID string, logger *slog.Logger) error {
	if err := msgbus.Publish(js, pipeline.JobCompleteMessage{JobID: jobID}, pubSubject); err != nil {
		logger.Error("failed to pub msg for video convert complete", "jobID", jobID, "err", err)
		return err
	}

	return msgbus.MarkProcessed(processedKV, jobID, logger)
}
