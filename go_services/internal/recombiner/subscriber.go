package recombiner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"splice.com/go_services/internal/shared/msgbus"
	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/storage"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.chunks.complete"
const uploadTimeout, downloadTimeout = 30 * time.Second, 30 * time.Second

// recombines video chunks back into one video
func RecombineVideo(
	js jetstream.JetStream, nc pipeline.Publisher,
	msgRecievedKV, jobMilestoneKV, claimKV jetstream.KeyValue,
	ackWait time.Duration, logger *slog.Logger, baseStorageURL string,
) (jetstream.ConsumeContext, error) {
	return msgbus.ConsumeDurable(js, msgbus.ConsumerConfig{
		Subject:       subSubject,
		ServiceName:   "video-recombiner",
		AckWait:       ackWait,
		MaxAckPending: 10,
	}, func(msg jetstream.Msg) {
		payload, ok := msgbus.Decode[pipeline.ChunkCompleteMessage](msg, logger)
		if !ok {
			return
		}

		chunkProcessed, err := msgbus.IsProcessed(msgRecievedKV, payload.ChunkKVKey())
		if err != nil {
			logger.Error("failed to check chunk processed", "err", err)
			msgbus.NakWithErrHandling(logger, msg)
			return
		}
		if chunkProcessed {
			logger.Debug("chunk already, skipping")
			msgbus.AckWithErrHandling(logger, msg)
			return
		}

		_, stopJob := pipeline.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger)
		if stopJob {
			return
		}

		claimed, err := msgbus.ClaimAndRun(claimKV, payload.JobID, payload.ChunkIndex, logger, func() bool {
			shouldCancel, stopJob := pipeline.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger)
			if stopJob {
				return shouldCancel
			}

			ready, chunks, err := recordVideoChunkArrival(msgRecievedKV, payload, logger)
			if err != nil {
				msgbus.NakWithErrHandling(logger, msg)
				return false
			}
			if !ready {
				msgbus.AckWithErrHandling(logger, msg)
				return true
			}

			outputPath, err := recombineVideoChunks(nc, jobMilestoneKV, payload, chunks, logger)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					msgbus.NakWithErrHandling(logger, msg)
					return false
				}
				msgbus.NakWithErrHandling(logger, msg)
				return false
			}
			defer CleanUpTempFolders(payload.JobID, logger)

			fileName := filepath.Base(outputPath)
			url := fmt.Sprintf("%s/%s/%s/processed", baseStorageURL, payload.JobID, fileName)

			if err := storage.UploadFile(url, outputPath, uploadTimeout); err != nil {
				logger.Error("failed to upload recombined video", "job_id", payload.JobID, "err", err)
				msgbus.NakWithErrHandling(logger, msg)
				return false
			}

			shouldCancel, stopJob = pipeline.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger)
			if stopJob {
				return shouldCancel
			}

			err = publishJetstreamCompleteMsg(js, msgRecievedKV, payload, logger)
			if err != nil {
				msgbus.NakWithErrHandling(logger, msg)
				return false
			}
			msgbus.AckWithErrHandling(logger, msg)
			return true
		})
		if err != nil {
			logger.Error("failed to claim chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
			msgbus.NakWithErrHandling(logger, msg)
			return
		}
		if !claimed {
			logger.Debug("chunk already claimed by another worker, skipping...", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex)
			return
		}
	})
}

// records this video chunk and returns whether all chunks for a single jobID is recieved and good for processing
// returns a ready bool, the map of chunks, and the error
func recordVideoChunkArrival(msgRecievedKV jetstream.KeyValue, payload pipeline.ChunkCompleteMessage, logger *slog.Logger) (bool, map[int]string, error) {
	ready, chunks, err := Add(msgRecievedKV, payload, logger)
	if err != nil {
		logger.Error("failed to record chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		return false, nil, err
	}
	if !ready {
		if err := msgbus.MarkProcessed(msgRecievedKV, payload.ChunkKVKey(), logger); err != nil {
			return false, nil, err
		}
		return false, nil, nil
	}

	return true, chunks, nil
}

// only runs once every video chunk has been recieved and recombines it into one full video
// returns the outputPath string and error
func recombineVideoChunks(
	nc pipeline.Publisher, jobMilestoneKV jetstream.KeyValue, payload pipeline.ChunkCompleteMessage, chunks map[int]string, logger *slog.Logger,
) (string, error) {
	if err := pipeline.AdvanceMilestone(jobMilestoneKV, payload.JobID, pipeline.JobStatus{State: "PROCESSING", Stage: "video-recombiner"}); err != nil {
		logger.Error("failed to update job-milestones stage", "job_id", payload.JobID, "err", err)
		return "", err
	}

	localChunks := make(map[int]string)
	var downloadErr error

	for idx, storageURL := range chunks {
		fileName := fmt.Sprintf("processed_chunk-%s", payload.JobID)

		localPath, err := storage.DownloadVideo(storageURL, fileName, downloadTimeout)
		if err != nil {
			logger.Error("failed to download video chunk", "job_id", payload.JobID, "chunk_index", idx, "err", err)
			downloadErr = err
			break
		}
		localChunks[idx] = localPath
	}
	if downloadErr != nil {
		CleanUpTempFolders(payload.JobID, logger)
		return "", downloadErr
	}

	onProgress := pipeline.NewProgressReporter(nc, payload.JobID, "video-recombiner", logger)
	outputPath, err := CombineChunks(payload.JobID, localChunks, onProgress)
	if err != nil {
		logger.Error("failed to combine chunks", "job_id", payload.JobID, "err", err)
		CleanUpTempFolders(payload.JobID, logger)
		return "", err
	}

	return outputPath, nil
}
