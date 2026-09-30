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
	js jetstream.JetStream, nc handler.Publisher,
	msgRecievedKV, jobMilestoneKV, claimKV jetstream.KeyValue,
	ackWait time.Duration, logger *slog.Logger, baseStorageURL string,
) (jetstream.ConsumeContext, error) {
	maxAckPending := 10
	cons, err := sJetstream.CreateDurableConsumer(js, subSubject, "video-recombiner", ackWait, maxAckPending)
	if err != nil {
		return nil, err
	}

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		payload, ok := sJetstream.UnmarshalJetstreamMsg[handler.ChunkCompleteMessage](msg, logger)
		if !ok {
			return
		}

		recieved, err := sJetstream.CheckKeyExist(msgRecievedKV, payload.ChunkKVKey())
		if err != nil {
			logger.Error("failed to check chunk recieved", "err", err)
			return
		}
		if recieved {
			logger.Debug("message already recieved, skipping")
			sJetstream.AckWithErrHandling(logger, msg)
			return
		}

		_, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger)
		if stopJob {
			return
		}

		claimed, err := sJetstream.ClaimAndRun(claimKV, payload.JobID, payload.ChunkIndex, logger, func() bool {
			shouldCancel, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger)
			if stopJob {
				return shouldCancel
			}

			ready, chunks, err := recordVideoChunkArrival(msgRecievedKV, payload, logger)
			if err != nil {
				sJetstream.NakWithErrHandling(logger, msg)
				return false
			}
			if !ready {
				sJetstream.AckWithErrHandling(logger, msg)
				return true
			}

			outputPath, err := recombineVideoChunks(nc, jobMilestoneKV, payload, chunks, logger)
			if err != nil {
				sJetstream.NakWithErrHandling(logger, msg)
				return false
			}
			defer CleanUpTempFolders(payload.JobID, logger)

			fileName := filepath.Base(outputPath)
			url := fmt.Sprintf("%s/%s/%s/processed", baseStorageURL, payload.JobID, fileName)

			_, err = storage.UploadVideoChunk(url, outputPath)
			if err != nil {
				logger.Error("failed to upload recombined video", "job_id", payload.JobID, "err", err)
				sJetstream.NakWithErrHandling(logger, msg)
				return false
			}

			shouldCancel, stopJob = sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger)
			if stopJob {
				return shouldCancel
			}

			err = publishJetstreamCompleteMsg(js, msgRecievedKV, payload, logger)
			if err != nil {
				sJetstream.NakWithErrHandling(logger, msg)
				return false
			}
			sJetstream.AckWithErrHandling(logger, msg)
			return true
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

// records this video chunk and returns whether all chunks for a single jobID is recieved and good for processing
// returns a ready bool, the map of chunks, and the error
func recordVideoChunkArrival(msgRecievedKV jetstream.KeyValue, payload handler.ChunkCompleteMessage, logger *slog.Logger) (bool, map[int]string, error) {
	ready, chunks, err := Add(msgRecievedKV, payload, logger)
	if err != nil {
		logger.Error("failed to record chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		return false, nil, err
	}
	if !ready {
		if err := sJetstream.MarkChunkProcessed(msgRecievedKV, payload.ChunkKVKey(), logger); err != nil {
			return false, nil, err
		}
		return false, nil, nil
	}

	return true, chunks, nil
}

// only runs once every video chunk has been recieved and recombines it into one full video
// returns the outputPath string and error
func recombineVideoChunks(
	nc handler.Publisher, jobMilestoneKV jetstream.KeyValue, payload handler.ChunkCompleteMessage, chunks map[int]string, logger *slog.Logger,
) (string, error) {
	err := sJetstream.AdvanceMilestone(jobMilestoneKV, payload.JobID, sJetstream.JobStatus{State: "PROCESSING", Stage: "video-recombiner"})
	if err != nil {
		logger.Error("failed to update job-milestones stage", "job_id", payload.JobID, "err", err)
		return "", err
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
		return "", downloadErr
	}

	onProgress := handler.NewProgressReporter(nc, payload.JobID, "video-recombiner", logger)
	outputPath, err := CombineChunks(payload.JobID, localChunks, onProgress)
	if err != nil {
		logger.Error("failed to combine chunks", "job_id", payload.JobID, "err", err)
		CleanUpTempFolders(payload.JobID, logger)
		return "", err
	}

	return outputPath, nil
}
