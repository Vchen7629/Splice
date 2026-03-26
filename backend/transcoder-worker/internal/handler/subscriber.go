package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
	"transcoder-worker/internal/service"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.video.chunks"

// consume video chunk from nats jetstream and process it
func ConsumeVideoChunk(js jetstream.JetStream, logger *slog.Logger, outputDir string) (jetstream.ConsumeContext, error) {
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
		Name:          "transcoder-worker",
		Durable:       "transcoder-worker",
		Description:   "takes in nats msgs with job metadata and transcodes the video chunk",
		FilterSubject: subSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxAckPending: 10, // worker wont recieve more than 10 inflight messages
		MaxDeliver:    3,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		var payload service.VideoChunkMessage

		err := json.Unmarshal(msg.Data(), &payload)
		if err != nil {
			logger.Error("failed to unmarshal msg from jetstream", "err", err)
			err := msg.Nak()
			if err != nil {
				logger.Error("error naking msg", "err", err)
				return
			}
			return
		}

		outputPath, err := service.TranscodeVideo(payload, outputDir, logger)
		if err != nil {
			logger.Error("error transcoding chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
			err := msg.Nak()
			if err != nil {
				logger.Error("error naking msg", "err", err)
				return
			}
			return
		}

		err = PublishChunkComplete(js)(service.ChunkCompleteMessage{
			JobID:      payload.JobID,
			ChunkIndex: payload.ChunkIndex,
			OutputPath: outputPath,
		})
		if err != nil {
			logger.Error("failed to pub chunk complete msg", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
			err := msg.Nak()
			if err != nil {
				logger.Error("error naking msg", "err", err)
				return
			}
			return
		}

		err = msg.Ack()
		if err != nil {
			logger.Error("error acking msg", "err", err)
			return
		}
	})
	if err != nil {
		return nil, err
	}

	return consCtx, nil
}
