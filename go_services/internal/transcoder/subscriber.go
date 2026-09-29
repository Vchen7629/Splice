package transcoder

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/storage"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.video.chunks"

// removeAll and transcodeVideo are variables for the tests to override to mock
var removeAll = os.RemoveAll
var transcodeVideo = TranscodeVideo

// consume video chunk from nats jetstream and process it
func ConsumeVideoChunk(
	baseStorageURL string, nc handler.Publisher,
	js jetstream.JetStream, processedKV, jobMilestoneKV, claimKV jetstream.KeyValue,
	ackWait time.Duration, logger *slog.Logger,
) (jetstream.ConsumeContext, error) {
	maxAckPending := 10
	cons, err := sJetstream.CreateDurableConsumer(js, subSubject, "transcoder-worker", ackWait, maxAckPending)
	if err != nil {
		return nil, err
	}

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		payload, ok := sJetstream.UnmarshalJetstreamMsg[VideoChunkMessage](msg, logger)
		if !ok {
			return
		}

		processed, err := sJetstream.CheckKeyExist(processedKV, fmt.Sprintf("%s.%d", payload.JobID, payload.ChunkIndex))
		if err != nil {
			logger.Error("failed to check chunk processed", "err", err)
			return
		}
		if processed {
			logger.Debug("message already processed, skipping")
			sJetstream.AckWithErrHandling(logger, msg)
			return
		}

		_, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger)
		if stopJob {
			return
		}

		claimed, err := sJetstream.ClaimAndRun(claimKV, payload.JobID, payload.ChunkIndex, logger, func() bool {
			chunkName := fmt.Sprintf("%s-%d", payload.JobID, payload.ChunkIndex)
			defer cleanupTempFolders(chunkName, logger)

			shouldCancel, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger)
			if stopJob {
				return shouldCancel
			}

			outputPath, err := processChunk(jobMilestoneKV, payload, logger)
			if err != nil {
				sJetstream.NakWithErrHandling(logger, msg)
				return false
			}

			outFileName := filepath.Base(outputPath)
			url := fmt.Sprintf("%s/%s/processed/%s", baseStorageURL, payload.JobID, outFileName)

			storageURL, err := storage.UploadVideoChunk(url, outputPath)
			if err != nil {
				logger.Error(
					"error saving transcoded video chunk to seaweedfs storage",
					"job_id", payload.JobID,
					"file_path", outputPath,
					"err", err,
				)
				sJetstream.NakWithErrHandling(logger, msg)
				return false
			}

			shouldCancel, stopJob = sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger)
			if stopJob {
				return shouldCancel
			}

			err = publishJetstreamProcessedMsg(nc, js, processedKV, payload, storageURL, logger)
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

// process (transcode) a video chunk from nats msg. Includes Updating the job status to be transcoder stage -> fetching the chunk
// -> transcoding it. returns the outputPath and the error
func processChunk(jobMilestoneKV jetstream.KeyValue, payload VideoChunkMessage, logger *slog.Logger) (string, error) {
	err := sJetstream.AdvanceMilestone(jobMilestoneKV, payload.JobID, sJetstream.JobStatus{State: "PROCESSING", Stage: "transcoder"})
	if err != nil {
		logger.Error("failed to update job-milestones stage", "job_id", payload.JobID, "err", err)
		return "", err
	}

	chunkName := fmt.Sprintf("%s-%d", payload.JobID, payload.ChunkIndex)

	filePath, err := storage.GetVideoChunk(payload.StorageURL, chunkName)
	if err != nil {
		logger.Error("error fetching unprocessed video chunk", "job_id", payload.JobID, "err", err)
		return "", err
	}

	outputPath, err := transcodeVideo(filePath, payload.TargetResolution, chunkName, logger)
	if err != nil {
		logger.Error("error transcoding chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		return "", err
	}

	return outputPath, nil
}

// cleanupTempFolders removes the unprocessed and processed temp dirs for a chunk.
// Must run after the chunk's output file is no longer needed (i.e. after upload).
func cleanupTempFolders(chunkName string, logger *slog.Logger) {
	if err := removeAll(storage.TempUnprocessedDir(chunkName)); err != nil {
		logger.Warn("error removing the temp unprocessed folder", "chunk_name", chunkName, "err", err)
	}
	if err := removeAll("/tmp/temp-processed-" + chunkName); err != nil {
		logger.Warn("error removing the temp processed folder", "chunk_name", chunkName, "err", err)
	}
}
