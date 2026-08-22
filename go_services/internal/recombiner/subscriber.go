package recombiner

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/storage"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.chunks.complete"

// recombines video chunks back into one video
func RecombineVideo(
	js jetstream.JetStream, msgRecievedKV, jobStatusKV, claimKV jetstream.KeyValue,
	ackWait time.Duration, logger *slog.Logger, baseStorageURL string,
) (jetstream.ConsumeContext, error) {
	cons, err := sJetstream.CreateDurableConsumer(js, subSubject, "video-recombiner", ackWait)
	if err != nil {
		return nil, err
	}

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		payload, ok := sJetstream.UnmarshalJetstreamMsg[handler.ChunkCompleteMessage](msg, logger)
		if !ok {
			return
		}

		recieved, err := sJetstream.CheckKeyExist(msgRecievedKV, fmt.Sprintf("%s.%d", payload.JobID, payload.ChunkIndex))
		if err != nil {
			logger.Error("failed to check chunk recieved", "err", err)
			return
		}

		if recieved {
			logger.Debug("message already recieved, skipping")
			sJetstream.AckWithErrHandling(logger, msg)
			return
		}

		claimed, err := sJetstream.ClaimAndRun(claimKV, payload.JobID, payload.ChunkIndex, logger, func() bool {
			return recombineChunks(js, jobStatusKV, msgRecievedKV, msg, payload, baseStorageURL, logger)
		})
		if err != nil {
			logger.Error("failed to claim chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
			sJetstream.NakWithErrHandling(logger, msg)
			return
		}
		if !claimed {
			logger.Debug("chunk already claimed by another worker, skipping...", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex)
			return
		}
	})
	if err != nil {
		return nil, err
	}

	return consCtx, nil
}

// recombine video chunks from nats msgs. Includes Updating the job status to be video-recombiner stage -> fetching chunks
// -> recombining all the chunks for a video -> writing/uploading the processed chunsk to seaweedfs storage -> cleanup temp files
// -> publishing the job complete nats msg
// returns a bool: false if any part fails and we want to stop or true if its done
func recombineChunks(
	js jetstream.JetStream, jobStatusKV, msgRecievedKV jetstream.KeyValue, msg jetstream.Msg,
	payload handler.ChunkCompleteMessage, baseStorageURL string, logger *slog.Logger,
) bool {
	processingMsg, err := handler.MarshalProcessingStatusMsg("video-recombiner")
	if err != nil {
		logger.Error("error marshalling status text", "err", err)
	}
	err = sJetstream.PutKeyKV(jobStatusKV, payload.JobID, processingMsg)
	if err != nil {
		logger.Error("failed to update job_status stage", "job_id", payload.JobID, "err", err)
	}

	ready, chunks, err := Add(msgRecievedKV, payload, logger)
	if err != nil {
		logger.Error("failed to record chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	// not the triggering chunk: nothing to combine yet, so this chunk's message is fully handled now
	if !ready {
		err = sJetstream.PutKeyKV(msgRecievedKV, fmt.Sprintf("%s.%d", payload.JobID, payload.ChunkIndex), []byte("processed"))
		if err != nil {
			logger.Error("failed to mark job chunk as recieved", "err", err)
			sJetstream.NakWithErrHandling(logger, msg)
			return false
		}

		sJetstream.AckWithErrHandling(logger, msg)
		return true
	}

	localChunks := make(map[int]string)
	var downloadErr error

	for idx, storageURL := range chunks {
		fileName := fmt.Sprintf("processed_chunk-%s", payload.JobID)

		localPath, err := storage.GetVideoChunk(storageURL, fileName)
		if err != nil {
			logger.Error("failed to download chunk", "job_id", payload.JobID, "chunk_index", idx, "err", err)
			downloadErr = err
			break
		}
		localChunks[idx] = localPath
	}
	if downloadErr != nil {
		CleanUpTempFolders(payload.JobID, logger)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	outputPath, err := CombineChunks(payload.JobID, localChunks)
	if err != nil {
		logger.Error("failed to combine chunks", "job_id", payload.JobID, "err", err)
		CleanUpTempFolders(payload.JobID, logger)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	fileName := filepath.Base(outputPath)
	url := fmt.Sprintf("%s/%s/%s/processed", baseStorageURL, payload.JobID, fileName)

	_, err = storage.UploadVideoChunk(url, outputPath)
	if err != nil {
		logger.Error("failed to upload recombined video", "job_id", payload.JobID, "err", err)
		CleanUpTempFolders(payload.JobID, logger)
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

	logger.Debug("job complete", "job_id", payload.JobID, "output_path", outputPath)

	const pubSubject = "jobs.complete"
	err = sJetstream.PublishJetstreamMsg(js, handler.JobCompleteMessage{JobID: payload.JobID}, pubSubject)
	if err != nil {
		logger.Error("failed to pub msg for video processing complete", "job_id", payload.JobID, "err", err)
	}

	return true
}
