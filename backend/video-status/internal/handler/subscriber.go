package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"shared/handler"
	"shared/kv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const advisorySubject = "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.>"

type maxDeliveryAdvisory struct {
	Stream    string `json:"stream"`
	Consumer  string `json:"consumer"`
	StreamSeq uint64 `json:"stream_seq"`
}

// extracts job_id from original msg payload
// all job msgs across services embed job_id at top level
type jobIDPayload struct {
	JobID string `json:"job_id"`
}

// subscribes to Jetstream max delivery advisories on core NATS, fetches the original msg to get job ID, writes FAILED to KV bucket
func ListenAdvisoriesFailure(nc *nats.Conn, js jetstream.JetStream, kv jetstream.KeyValue, logger *slog.Logger) (*nats.Subscription, error) {
	sub, err := nc.Subscribe(advisorySubject, func(msg *nats.Msg) {
		var advisory maxDeliveryAdvisory

		err := json.Unmarshal(msg.Data, &advisory)
		if err != nil {
			logger.Error("failed to unmarshal advisory", "err", err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		// fetch original msg from the stream to get job ID
		stream, err := js.Stream(ctx, advisory.Stream)
		if err != nil {
			logger.Error("no stream found for subject", "subject", advisory.Stream, "err", err)
			return
		}

		rawMsg, err := stream.GetMsg(ctx, advisory.StreamSeq)
		if err != nil {
			logger.Error("failed to fetch original message", "stream", advisory.Stream, "seq", advisory.StreamSeq, "err", err)
			return
		}

		var payload jobIDPayload
		err = json.Unmarshal(rawMsg.Data, &payload)
		if err != nil {
			logger.Error("failed to unmarshal job id from original msg", "err", err)
			return
		}

		status, err := json.Marshal(JobStatus{
			State: StateFailed,
			Error: fmt.Sprintf("pipeline failed at stage: %s", advisory.Consumer),
		})
		if err != nil {
			logger.Error("failed to marshal failed status", "err", err)
			return
		}

		_, err = kv.Put(ctx, payload.JobID, status)
		if err != nil {
			logger.Error("failed to write failed status to kv", "job_id", payload.JobID, "err", err)
			return
		}
	})

	return sub, err
}

// subs to jobs.complete (from video-recombiner service) via jetstream consumer and writes COMPLETE to KV
func ListenJobComplete(js jetstream.JetStream, jobStatusKV jetstream.KeyValue, logger *slog.Logger) (jetstream.ConsumeContext, error) {
	ctx := context.Background()

	streamName, err := js.StreamNameBySubject(ctx, "jobs.complete")
	if err != nil {
		return nil, fmt.Errorf("no stream found for jobs.complete: %w", err)
	}

	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return nil, err
	}

	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          "video-status-complete",
		Durable:       "video-status-complete",
		FilterSubject: "jobs.complete",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    3,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		payload, ok := handler.UnmarshalJetstreamMsg[jobIDPayload](msg, logger)
		if !ok {
			return
		}

		status, err := json.Marshal(JobStatus{State: StateComplete})
		if err != nil {
			logger.Error("failed to marshal complete status", "err", err)
			kv.NakWithErrHandling(logger, msg)
			return
		}

		_, err = jobStatusKV.Put(context.Background(), payload.JobID, status)
		if err != nil {
			logger.Error("failed to write complete status to kv", "job_id", payload.JobID, "err", err)
			kv.NakWithErrHandling(logger, msg)
			return
		}

		logger.Debug("job marked as complete", "job_id", payload.JobID)
		err = msg.Ack()
		if err != nil {
			logger.Error("failed to ack message after put kv", "err", err)
			return
		}
	})

	return consCtx, err
}
