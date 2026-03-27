package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
	"video-upload/internal/service"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.complete"

// SubscribeJobCompletion starts a durable consumer on jobs.complete and populates
// the tracker as jobs finish. Returns a ConsumeContext for graceful shutdown.
func SubscribeJobCompletion(js jetstream.JetStream, logger *slog.Logger) (*service.CompletedJobs, jetstream.ConsumeContext, error) {
	ctx := context.Background()

	streamName, err := js.StreamNameBySubject(ctx, subSubject)
	if err != nil {
		return nil, nil, fmt.Errorf("no stream found for subject %s: %w", subSubject, err)
	}

	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return nil, nil, err
	}

	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          "video-upload-status",
		Durable:       "video-upload-status",
		Description:   "tracks completed job IDs for status polling",
		FilterSubject: subSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxAckPending: 50,
		MaxDeliver:    3,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return nil, nil, err
	}

	tracker := service.NewCompletedJobs()

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		var payload service.JobCompleteMessage

		if err := json.Unmarshal(msg.Data(), &payload); err != nil {
			logger.Error("failed to unmarshal job complete msg", "err", err)
			if err := msg.Nak(); err != nil {
				logger.Error("error naking msg", "err", err)
			}
			return
		}

		tracker.AddJob(payload.JobID)
		logger.Debug("job marked complete", "job_id", payload.JobID)

		if err := msg.Ack(); err != nil {
			logger.Error("error acking msg", "err", err)
		}
	})
	if err != nil {
		return nil, nil, err
	}

	return tracker, consCtx, nil
}
