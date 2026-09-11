package transcoder

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/storage"
)

// uploads the recombined video to seaweedfs storage and naks the msg on failure.
func uploadVideoChunk(msg jetstream.Msg, outputPath, baseStorageURL, jobID string, logger *slog.Logger) (bool, string) {
	outFileName := filepath.Base(outputPath)
	url := fmt.Sprintf("%s/%s/processed/%s", baseStorageURL, jobID, outFileName)

	storageUrl, err := storage.UploadVideoChunk(url, outputPath)
	if err != nil {
		logger.Error(
			"error saving transcoded video chunk to seaweedfs storage",
			"job_id", jobID,
			"file_path", outputPath,
			"err", err,
		)
		sJetstream.NakWithErrHandling(logger, msg)
		return false, ""
	}

	return true, storageUrl
}

// publishes the "processed" jetstream msg, updates the KeyValue, and updates Job Progress KV
func publishJetstreamProcessedMsg(
	nc handler.Publisher, js jetstream.JetStream, processedKV jetstream.KeyValue, msg jetstream.Msg,
	payload VideoChunkMessage, storageUrl string, logger *slog.Logger,
) bool {
	const pubSubject = "jobs.chunks.complete"

	err := sJetstream.PublishJetstreamMsg(js, handler.ChunkCompleteMessage{
		JobID:       payload.JobID,
		ChunkIndex:  payload.ChunkIndex,
		TotalChunks: payload.TotalChunks,
		StorageURL:  storageUrl,
	}, pubSubject)
	if err != nil {
		logger.Error("failed to pub chunk complete msg", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	err = sJetstream.PutKeyKV(processedKV, fmt.Sprintf("%s.%d", payload.JobID, payload.ChunkIndex), []byte("processed"))
	if err != nil {
		logger.Error("failed to mark job chunk as processed", "err", err)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	pct, err := jobProgressPct(context.Background(), processedKV, payload.JobID, payload.TotalChunks, logger)
	if err != nil {
		logger.Error("failed to compute job progress", "job_id", payload.JobID, "err", err)
	} else {
		handler.NewProgressReporter(nc, payload.JobID, "transcoder", logger)(pct)
	}

	err = msg.Ack()
	if err != nil {
		logger.Error("error acking msg", "err", err)
		return true
	}

	return true
}
