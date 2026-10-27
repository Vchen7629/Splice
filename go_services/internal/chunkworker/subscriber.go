package chunkworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"splice.com/go_services/internal/shared/msgbus"
	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/storage"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.video.chunks"
const uploadTimeout = 30 * time.Second

var removeAll = os.RemoveAll

// consume video chunk from nats jetstream and process it
func ConsumeVideoChunk(
	baseStorageURL string, nc pipeline.Publisher,
	js jetstream.JetStream, processedKV, jobMilestoneKV, claimKV jetstream.KeyValue,
	ackWait time.Duration, logger *slog.Logger,
) (jetstream.ConsumeContext, error) {
	return msgbus.ConsumeDurable(js, msgbus.ConsumerConfig{
		Subject:       subSubject,
		ServiceName:   "chunk-worker",
		AckWait:       ackWait,
		MaxAckPending: 10,
	}, func(msg jetstream.Msg) {
		payload, ok := msgbus.Decode[VideoChunkMessage](msg, logger)
		if !ok {
			return
		}

		processed, err := msgbus.IsProcessed(processedKV, payload.ChunkKVKey())
		if err != nil {
			logger.Error("failed to check chunk processed", "err", err)
			msgbus.NakWithErrHandling(logger, msg)
			return
		}
		if processed {
			logger.Debug("message already processed, skipping")
			msgbus.AckWithErrHandling(logger, msg)
			return
		}

		if _, stopJob := pipeline.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
			return
		}

		claimed, err := msgbus.ClaimAndRun(claimKV, payload.JobID, payload.ChunkIndex, logger, func() bool {
			chunkName := fmt.Sprintf("%s-%d", payload.JobID, payload.ChunkIndex)
			defer cleanupTempFolders(chunkName, logger)

			if shouldCancel, stopJob := pipeline.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
				return shouldCancel
			}

			outputPath, err := processChunk(jobMilestoneKV, payload, logger)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					msgbus.NakWithErrHandling(logger, msg)
					return false
				}
				msgbus.NakWithErrHandling(logger, msg)
				return false
			}

			if shouldCancel, stopJob := pipeline.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
				return shouldCancel
			}

			outFileName := filepath.Base(outputPath)
			storageURL := fmt.Sprintf("%s/%s/processed/%s", baseStorageURL, payload.JobID, outFileName)

			if err := storage.UploadFile(storageURL, outputPath, uploadTimeout); err != nil {
				logger.Error(
					"error saving transcoded video chunk to seaweedfs storage",
					"job_id", payload.JobID,
					"file_path", outputPath,
					"err", err,
				)
				msgbus.NakWithErrHandling(logger, msg)
				return false
			}

			if shouldCancel, stopJob := pipeline.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
				return shouldCancel
			}

			if err := publishJetstreamProcessedMsg(nc, js, processedKV, payload, storageURL, logger); err != nil {
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

// cleanupTempFolders removes the unprocessed and processed temp dirs for a chunk.
// Must run after the chunk's output file is no longer needed (i.e. after upload).
func cleanupTempFolders(chunkName string, logger *slog.Logger) {
	if err := removeAll(storage.TempUnprocessedDir(chunkName)); err != nil {
		logger.Warn("error removing the temp unprocessed folder", "chunk_name", chunkName, "err", err)
	}
	if err := removeAll("/tmp/transcode/temp-processed-" + chunkName); err != nil {
		logger.Warn("error removing the temp transcode processed folder", "chunk_name", chunkName, "err", err)
	}
	if err := removeAll("/tmp/denoise/temp-processed-" + chunkName); err != nil {
		logger.Warn("error removing the temp denoise processed folder", "chunk_name", chunkName, "err", err)
	}
}
