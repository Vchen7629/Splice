package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"
	"video-recombiner/internal/service"
	"video-recombiner/internal/storage"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.chunks.complete"

var removeAll = os.RemoveAll

// recombines video chunks back into one video
func RecombineVideo(js jetstream.JetStream, logger *slog.Logger, baseStorageURL string) (jetstream.ConsumeContext, error) {
	ctx := context.Background()

	streamName, err := js.StreamNameBySubject(ctx, subSubject)
	if err != nil {
		return nil, fmt.Errorf("no stream found for subject: %s: %w", subSubject, err)
	}

	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return nil, err
	}

	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          "video-recombiner",
		Durable:       "video-recombiner",
		Description:   "takes in nats msgs with video chunks and recombines it once it gathered all chunks",
		FilterSubject: subSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxAckPending: 10, // worker wont recieve more than 10 inflight messages
		MaxDeliver:    3,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	tracker := service.NewJobTracker()

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		var payload service.ChunkCompleteMessage

		err := json.Unmarshal(msg.Data(), &payload)
		if err != nil {
			logger.Error("failed to unmarshal msg from jetstream", "err", err)
			if err := msg.Nak(); err != nil {
				logger.Error("error naking msg", "err", err)
			}
			return
		}

		ready, chunks := tracker.Add(payload.JobID, payload.ChunkIndex, payload.StorageURL, payload.TotalChunks)

		err = msg.Ack()
		if err != nil {
			logger.Error("error acking msg", "err", err)
			return
		}

		if ready {
			localChunks := make(map[int]string)
			failed := false

			for idx, storageURL := range chunks {
				localPath, err := storage.GetProcessedVideoChunk(storageURL, payload.JobID)
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

			_, err = storage.UploadRecombinedVideo(baseStorageURL, outputPath, payload.JobID)
			if err != nil {
				logger.Error("failed to upload recombined video", "job_id", payload.JobID, "err", err)
				return
			}

			err = removeAll("/tmp/processed_chunk-" + payload.JobID)
			if err != nil {
				logger.Warn("failed to clean up chunk temp dir", "job_id", payload.JobID, "err", err)
			}

			err = removeAll("/tmp/jobs/" + payload.JobID)
			if err != nil {
				logger.Warn("failed to clean up job temp dir", "job_id", payload.JobID, "err", err)
			}

			logger.Debug("job complete", "job_id", payload.JobID, "output_path", outputPath)
			if err := PublishVideoProcessingComplete(js, service.VideoProcessingCompleteMessage{JobID: payload.JobID}); err != nil {
				logger.Error("failed to pub msg for video processing complete", "job_id", payload.JobID, "err", err)
			}
		}
	})
	if err != nil {
		return nil, err
	}

	return consCtx, nil
}
