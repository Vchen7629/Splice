package handler

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"shared/handler"
	"shared/kv"
	"shared/storage"
	"transcoder-worker/internal/service"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.video.chunks"

// removeAll is a variable so tests can override it to simulate filesystem failures.
var removeAll = os.RemoveAll

// consume video chunk from nats jetstream and process it
func ConsumeVideoChunk(
	baseStorageURL string, js jetstream.JetStream, processedKV, jobStatusKV jetstream.KeyValue, logger *slog.Logger,
) (jetstream.ConsumeContext, error) {
	cons, err := handler.CreateDurableConsumer(js, subSubject, "transcoder-worker")
	if err != nil {
		return nil, err
	}

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		payload, ok := handler.UnmarshalJetstreamMsg[service.VideoChunkMessage](msg, logger)
		if !ok {
			return
		}

		exists, err := kv.CheckChunkProcessed(processedKV, payload.JobID, payload.ChunkIndex)
		if err != nil {
			logger.Error("failed to check chunk processed", "err", err)
			return
		}

		if exists {
			logger.Debug("message already processed, skipping")
			kv.AckWithErrHandling(logger, msg)
			return
		}

		err = kv.UpdateJobStatus(jobStatusKV, "transcoder", payload.JobID, logger)
		if err != nil {
			logger.Error("failed to update job_status stage", "job_id", payload.JobID, "err", err)
		}

		fileName := fmt.Sprintf("temp-unprocessed-%s", payload.JobID)

		filePath, err := storage.GetVideoChunk(payload.StorageURL, fileName)
		if err != nil {
			logger.Error("error fetching unprocessed video chunk", "job_id", payload.JobID, "err", err)
			kv.NakWithErrHandling(logger, msg)
			return
		}

		outputPath, err := service.TranscodeVideo(filePath, payload.TargetResolution, payload.JobID, logger)
		if err != nil {
			logger.Error("error transcoding chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
			kv.NakWithErrHandling(logger, msg)
			return
		}

		fileName = filepath.Base(outputPath)
		url := fmt.Sprintf("%s/%s/processed/%s", baseStorageURL, payload.JobID, fileName)

		storageUrl, err := storage.UploadVideoChunk(url, outputPath)
		if err != nil {
			logger.Error(
				"error saving transcoded video chunk to seaweedfs storage",
				"job_id", payload.JobID,
				"file_path", outputPath,
				"err", err,
			)
			kv.NakWithErrHandling(logger, msg)
			return
		}

		const pubSubject = "jobs.chunks.complete"

		err = handler.PublishJobComplete(js, handler.ChunkCompleteMessage{
			JobID:       payload.JobID,
			ChunkIndex:  payload.ChunkIndex,
			TotalChunks: payload.TotalChunks,
			StorageURL:  storageUrl,
		}, pubSubject)
		if err != nil {
			logger.Error("failed to pub chunk complete msg", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
			kv.NakWithErrHandling(logger, msg)
			return
		}

		err = msg.Ack()
		if err != nil {
			logger.Error("error acking msg", "err", err)
			return
		}

		err = kv.AddChunkProcessed(processedKV, payload.JobID, payload.ChunkIndex)
		if err != nil {
			logger.Error("failed to mark job chunk as processed", "err", err)
			return
		}

		err = removeAll("/tmp/temp-unprocessed-" + payload.JobID)
		if err != nil {
			logger.Warn("error removing the temp unprocessed folder", "err", err)
			return
		}
		err = removeAll("/tmp/temp-processed-" + payload.JobID)
		if err != nil {
			logger.Warn("error removing the temp unprocessed folder", "err", err)
			return
		}
	})
	if err != nil {
		return nil, err
	}

	return consCtx, nil
}
