package chunkworker

import (
	"context"
	"errors"
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

var removeAll = os.RemoveAll

// consume video chunk from nats jetstream and process it
func ConsumeVideoChunk(
	baseStorageURL string, nc handler.Publisher,
	js jetstream.JetStream, processedKV, jobMilestoneKV, claimKV jetstream.KeyValue,
	ackWait time.Duration, logger *slog.Logger,
) (jetstream.ConsumeContext, error) {
	maxAckPending := 10
	cons, err := sJetstream.CreateDurableConsumer(js, subSubject, "chunk-worker", ackWait, maxAckPending)
	if err != nil {
		return nil, err
	}

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		payload, ok := sJetstream.UnmarshalJetstreamMsg[VideoChunkMessage](msg, logger)
		if !ok {
			return
		}

		processed, err := sJetstream.CheckKeyExist(processedKV, payload.ChunkKVKey())
		if err != nil {
			logger.Error("failed to check chunk processed", "err", err)
			return
		}
		if processed {
			logger.Debug("message already processed, skipping")
			sJetstream.AckWithErrHandling(logger, msg)
			return
		}

		if _, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
			return
		}

		claimed, err := sJetstream.ClaimAndRun(claimKV, payload.JobID, payload.ChunkIndex, logger, func() bool {
			chunkName := fmt.Sprintf("%s-%d", payload.JobID, payload.ChunkIndex)
			defer cleanupTempFolders(chunkName, logger)

			if shouldCancel, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
				return shouldCancel
			}

			outputPath, err := processChunk(jobMilestoneKV, payload, logger)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					sJetstream.NakWithErrHandling(logger, msg)
					return false
				}
				sJetstream.NakWithErrHandling(logger, msg)
				return false
			}

			if shouldCancel, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
				return shouldCancel
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

			if shouldCancel, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
				return shouldCancel
			}

			if err := publishJetstreamProcessedMsg(nc, js, processedKV, payload, storageURL, logger); err != nil {
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
