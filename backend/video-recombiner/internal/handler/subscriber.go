package handler

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"shared/handler"
	"shared/kv"
	"shared/storage"
	"video-recombiner/internal/service"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.chunks.complete"

// recombines video chunks back into one video
func RecombineVideo(
	js jetstream.JetStream, msgRecievedKV, jobStatusKV jetstream.KeyValue, logger *slog.Logger, baseStorageURL string,
) (jetstream.ConsumeContext, error) {
	cons, err := handler.CreateDurableConsumer(js, subSubject, "video-recombiner")
	if err != nil {
		return nil, err
	}

	tracker := service.NewJobTracker()

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		payload, ok := handler.UnmarshalJetstreamMsg[handler.ChunkCompleteMessage](msg, logger)
		if !ok {
			return
		}

		recieved, err := kv.CheckChunkProcessed(msgRecievedKV, payload.JobID, payload.ChunkIndex)
		if err != nil {
			logger.Error("failed to check chunk recieved", "err", err)
			return
		}

		if recieved {
			logger.Debug("message already recieved, skipping")
			err := msg.Ack()
			if err != nil {
				logger.Error("error acking msg", "err", err)
				return
			}
			return
		}

		err = kv.UpdateJobStatus(jobStatusKV, "video-recombiner", payload.JobID, logger)
		if err != nil {
			logger.Error("failed to update job_status stage", "job_id", payload.JobID, "err", err)
		}

		ready, chunks := tracker.Add(payload.JobID, payload.ChunkIndex, payload.StorageURL, payload.TotalChunks)

		err = msg.Ack()
		if err != nil {
			logger.Error("error acking msg", "err", err)
			return
		}

		err = kv.AddChunkProcessed(msgRecievedKV, payload.JobID, payload.ChunkIndex)
		if err != nil {
			logger.Error("failed to mark job chunk as recieved", "err", err)
			return
		}

		if ready {
			localChunks := make(map[int]string)
			failed := false

			for idx, storageURL := range chunks {
				fileName := fmt.Sprintf("processed_chunk-%s", payload.JobID)

				localPath, err := storage.GetVideoChunk(storageURL, fileName)
				if err != nil {
					logger.Error("failed to download chunk", "job_id", payload.JobID, "chunk_index", idx, "err", err)
					failed = true
					break
				}
				localChunks[idx] = localPath
			}
			if failed {
				return
			}

			outputPath, err := service.CombineChunks(payload.JobID, localChunks)
			if err != nil {
				logger.Error("failed to combine chunks", "job_id", payload.JobID, "err", err)
				return
			}

			fileName := filepath.Base(outputPath)
			url := fmt.Sprintf("%s/%s/%s/processed", baseStorageURL, payload.JobID, fileName)

			_, err = storage.UploadVideoChunk(url, outputPath)
			if err != nil {
				logger.Error("failed to upload recombined video", "job_id", payload.JobID, "err", err)
				return
			}

			service.CleanUpTempFolders(payload.JobID, logger)

			logger.Debug("job complete", "job_id", payload.JobID, "output_path", outputPath)

			const pubSubject = "jobs.complete"
			err = handler.PublishJobComplete(js, handler.JobCompleteMessage{JobID: payload.JobID}, pubSubject)
			if err != nil {
				logger.Error("failed to pub msg for video processing complete", "job_id", payload.JobID, "err", err)
			}
		}
	})
	if err != nil {
		return nil, err
	}

	return consCtx, nil
}
