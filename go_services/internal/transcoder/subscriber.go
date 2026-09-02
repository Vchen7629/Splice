package transcoder

import (
	"context"
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
	cons, err := sJetstream.CreateDurableConsumer(js, subSubject, "transcoder-worker", ackWait)
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

		claimed, err := sJetstream.ClaimAndRun(claimKV, payload.JobID, payload.ChunkIndex, logger, func() bool {
			return processChunk(baseStorageURL, nc, js, processedKV, jobMilestoneKV, msg, payload, logger)
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
// -> transcoding it -> writing/uploading the processed chunk to seaweedfs storage -> publishing the transcode chunk complete to js
// -> acking and removing the temp files
// returns a bool: false if any part fails and we want to stop or true if its done
func processChunk(
	baseStorageURL string, nc handler.Publisher,
	js jetstream.JetStream, processedKV, jobMilestoneKV jetstream.KeyValue, msg jetstream.Msg,
	payload VideoChunkMessage, logger *slog.Logger,
) bool {
	err := sJetstream.AdvanceMilestone(jobMilestoneKV, payload.JobID, sJetstream.MilestoneStatus{State: "PROCESSING", Stage: "transcoder"})
	if err != nil {
		logger.Error("failed to update job-milestones stage", "job_id", payload.JobID, "err", err)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	chunkName := fmt.Sprintf("%s-%d", payload.JobID, payload.ChunkIndex)
	defer func() {
		err = removeAll("/tmp/temp-unprocessed-" + chunkName)
		if err != nil {
			logger.Warn("error removing the temp unprocessed folder", "err", err)
		}
		err = removeAll("/tmp/temp-processed-" + chunkName)
		if err != nil {
			logger.Warn("error removing the temp unprocessed folder", "err", err)
		}
	}()

	filePath, err := storage.GetVideoChunk(payload.StorageURL, chunkName)
	if err != nil {
		logger.Error("error fetching unprocessed video chunk", "job_id", payload.JobID, "err", err)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	outputPath, err := transcodeVideo(filePath, payload.TargetResolution, chunkName, logger)
	if err != nil {
		logger.Error("error transcoding chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		sJetstream.NakWithErrHandling(logger, msg)
		return false
	}

	outFileName := filepath.Base(outputPath)
	url := fmt.Sprintf("%s/%s/processed/%s", baseStorageURL, payload.JobID, outFileName)

	storageUrl, err := storage.UploadVideoChunk(url, outputPath)
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

	const pubSubject = "jobs.chunks.complete"

	err = sJetstream.PublishJetstreamMsg(js, handler.ChunkCompleteMessage{
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
