package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
	"video-recombiner/internal/service"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.chunks.complete"

// recombines video chunks back into one video
func RecombineVideo(js jetstream.JetStream, logger *slog.Logger, outputDIR string) (jetstream.ConsumeContext, error) {
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

		ready, chunks := tracker.Add(payload.JobID, payload.ChunkIndex, payload.OutputPath, payload.TotalChunks)

		err = msg.Ack()
		if err != nil {
			logger.Error("error acking msg", "err", err)
			return
		}

		if ready {
			outputPath, err := service.CombineChunks(payload.JobID, chunks, outputDIR)
			if err != nil {
				logger.Error("failed to combine chunks", "job_id", payload.JobID, "err", err)
				return
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
