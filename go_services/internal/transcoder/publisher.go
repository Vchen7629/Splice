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
func uploadVideoChunk(outputPath, baseStorageURL, jobID string, logger *slog.Logger) (string, error) {
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
		return "", err
	}

	return storageUrl, nil
}

// publishes the "processed" jetstream msg, updates the KeyValue, and updates Job Progress KV
func publishJetstreamProcessedMsg(
	nc handler.Publisher, js jetstream.JetStream, processedKV jetstream.KeyValue,
	payload VideoChunkMessage, storageUrl string, logger *slog.Logger,
) error {
	const pubSubject = "jobs.chunks.complete"

	err := sJetstream.PublishJetstreamMsg(js, handler.ChunkCompleteMessage{
		JobID:       payload.JobID,
		ChunkIndex:  payload.ChunkIndex,
		TotalChunks: payload.TotalChunks,
		StorageURL:  storageUrl,
	}, pubSubject)
	if err != nil {
		logger.Error("failed to pub chunk complete msg", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		return err
	}

	err = sJetstream.PutKeyKV(processedKV, fmt.Sprintf("%s.%d", payload.JobID, payload.ChunkIndex), []byte("processed"))
	if err != nil {
		logger.Error("failed to mark job chunk as processed", "err", err)
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
