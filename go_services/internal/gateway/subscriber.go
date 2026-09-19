package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	sJetstream "splice.com/go_services/internal/shared/jetstream"

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
func ListenAdvisoriesFailure(nc *nats.Conn, js jetstream.JetStream, jobMilestoneKV jetstream.KeyValue, logger *slog.Logger) (*nats.Subscription, error) {
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

		_, outcome, err := tryUpdateMilestone(ctx, jobMilestoneKV, payload.JobID, sJetstream.JobStatus{
			State: sJetstream.StateFailed,
			Error: fmt.Sprintf("pipeline failed at stage: %s", advisory.Consumer),
		})
		switch outcome {
		case milestoneSkipped:
			logger.Debug("job is already marked as terminal, skipping marking job as FAILED", "job_id", payload.JobID)
			return
		case milestoneNotFound:
			logger.Error("milestone entry not found for job, cannot mark FAILED", "job_id", payload.JobID)
			return
		case milestoneError:
			logger.Error("failed to write failed status to kv", "job_id", payload.JobID, "err", err)
			return
		}
	})

	return sub, err
}

// subs to jobs.complete (from video-recombiner service) via jetstream consumer and writes COMPLETE to KV
func ListenJobComplete(js jetstream.JetStream, jobMilestoneKV jetstream.KeyValue, logger *slog.Logger) (jetstream.ConsumeContext, error) {
	maxAckPending := -1 // set to -1 for infinite
	cons, err := sJetstream.CreateDurableConsumer(js, "jobs.complete", "video-status-complete", 30*time.Second, maxAckPending)
	if err != nil {
		return nil, err
	}

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		payload, ok := sJetstream.UnmarshalJetstreamMsg[jobIDPayload](msg, logger)
		if !ok {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, outcome, err := tryUpdateMilestone(ctx, jobMilestoneKV, payload.JobID, sJetstream.JobStatus{State: sJetstream.StateComplete})
		switch outcome {
		case milestoneSkipped:
			logger.Debug("job is already marked as terminal, skipping marking job as COMPLETE", "job_id", payload.JobID)
			sJetstream.AckWithErrHandling(logger, msg)
			return
		case milestoneNotFound:
			logger.Error("milestone entry not found for job, cannot mark COMPLETE", "job_id", payload.JobID)
			sJetstream.NakWithErrHandling(logger, msg)
			return
		case milestoneError:
			logger.Error("failed to write complete status to kv", "job_id", payload.JobID, "err", err)
			sJetstream.NakWithErrHandling(logger, msg)
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
