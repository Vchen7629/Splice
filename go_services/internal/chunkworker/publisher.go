package chunkworker

import (
	"context"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/msgbus"
	"splice.com/go_services/internal/shared/pipeline"
)

// publishes the "processed" jetstream msg, updates the KeyValue, and updates Job Progress KV
func publishJetstreamProcessedMsg(
	nc pipeline.Publisher, js jetstream.JetStream, processedKV jetstream.KeyValue,
	payload VideoChunkMessage, storageUrl string, logger *slog.Logger,
) error {
	const pubSubject = "jobs.chunks.complete"

	err := msgbus.Publish(js, pipeline.ChunkCompleteMessage{
		ChunkRef:    payload.ChunkRef,
		TotalChunks: payload.TotalChunks,
		StorageURL:  storageUrl,
	}, pubSubject)
	if err != nil {
		logger.Error("failed to pub chunk complete msg", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		return err
	}

	if err := msgbus.MarkProcessed(processedKV, payload.ChunkKVKey(), logger); err != nil {
		return err
	}

	pct, err := jobProgressPct(context.Background(), processedKV, payload.JobID, payload.TotalChunks, logger)
	if err != nil {
		logger.Error("failed to compute job progress", "job_id", payload.JobID, "err", err)
	} else {
		pipeline.NewProgressReporter(nc, payload.JobID, "chunk-worker", logger)(pct)
	}

	return nil
}
