package recombiner

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/storage"
)

// uploads the recombined video to seaweedfs storage. Cleans up temp files and naks the msg on failure.
func uploadVideoChunk(
	outputPath, baseStorageURL string, msg jetstream.Msg, payload handler.ChunkCompleteMessage, logger *slog.Logger,
) bool {
	fileName := filepath.Base(outputPath)
	url := fmt.Sprintf("%s/%s/%s/processed", baseStorageURL, payload.JobID, fileName)

	_, err := storage.UploadVideoChunk(url, outputPath)
	if err != nil {
		logger.Error("failed to upload recombined video", "job_id", payload.JobID, "err", err)
		CleanUpTempFolders(payload.JobID, logger)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	return true
}

func publishJetstreamCompleteMsg(
	js jetstream.JetStream, msgRecievedKV jetstream.KeyValue, msg jetstream.Msg, payload handler.ChunkCompleteMessage, logger *slog.Logger,
) bool {
	const pubSubject = "jobs.complete"
	err := sJetstream.PublishJetstreamMsg(js, handler.JobCompleteMessage{JobID: payload.JobID}, pubSubject)
	if err != nil {
		logger.Error("failed to pub msg for video processing complete", "job_id", payload.JobID, "err", err)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	err = sJetstream.PutKeyKV(msgRecievedKV, fmt.Sprintf("%s.%d", payload.JobID, payload.ChunkIndex), []byte("processed"))
	if err != nil {
		logger.Error("failed to mark job chunk as recieved", "err", err)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	sJetstream.AckWithErrHandling(logger, msg)
	CleanUpTempFolders(payload.JobID, logger)

	logger.Debug("job complete", "job_id", payload.JobID)

	return true
}
