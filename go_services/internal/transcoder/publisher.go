package transcoder

import (
	"context"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
)

// publishes the "processed" jetstream msg, updates the KeyValue, and updates Job Progress KV
func publishJetstreamProcessedMsg(
	nc handler.Publisher, js jetstream.JetStream, processedKV jetstream.KeyValue,
	payload VideoChunkMessage, storageUrl string, logger *slog.Logger,
) error {
	const pubSubject = "jobs.chunks.complete"

	err := sJetstream.PublishJetstreamMsg(js, handler.ChunkCompleteMessage{
		ChunkRef:    payload.ChunkRef,
		TotalChunks: payload.TotalChunks,
		StorageURL:  storageUrl,
	}, pubSubject)
	if err != nil {
		logger.Error("failed to pub chunk complete msg", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		return err
	}

	if err := sJetstream.MarkChunkProcessed(processedKV, payload.ChunkKVKey(), logger); err != nil {
		return err
	}

	pct, err := jobProgressPct(context.Background(), processedKV, payload.JobID, payload.TotalChunks, logger)
	if err != nil {
		logger.Error("failed to compute job progress", "job_id", payload.JobID, "err", err)
	} else {
		handler.NewProgressReporter(nc, payload.JobID, "transcoder", logger)(pct)
	}

	return nil
}
