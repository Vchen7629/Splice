package recombiner

import (
	"fmt"
	"log/slog"
	"time"

	"splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/storage"

	"github.com/nats-io/nats.go/jetstream"
)

const subSubject = "jobs.chunks.complete"

// recombines video chunks back into one video
func RecombineVideo(
	js jetstream.JetStream, nc handler.Publisher,
	msgRecievedKV, jobMilestoneKV, claimKV jetstream.KeyValue,
	ackWait time.Duration, logger *slog.Logger, baseStorageURL string,
) (jetstream.ConsumeContext, error) {
	cons, err := sJetstream.CreateDurableConsumer(js, subSubject, "video-recombiner", ackWait)
	if err != nil {
		return nil, err
	}

	consCtx, err := cons.Consume(func(msg jetstream.Msg) {
		payload, ok := sJetstream.UnmarshalJetstreamMsg[handler.ChunkCompleteMessage](msg, logger)
		if !ok {
			return
		}

		recieved, err := sJetstream.CheckKeyExist(msgRecievedKV, fmt.Sprintf("%s.%d", payload.JobID, payload.ChunkIndex))
		if err != nil {
			logger.Error("failed to check chunk recieved", "err", err)
			return
		}
		if recieved {
			logger.Debug("message already recieved, skipping")
			sJetstream.AckWithErrHandling(logger, msg)
			return
		}

		if terminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger) {
			return
		}

		claimed, err := sJetstream.ClaimAndRun(claimKV, payload.JobID, payload.ChunkIndex, logger, func() bool {
			if terminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger) {
				return true
			}

			recombined, outputPath := recombineChunks(nc, jobMilestoneKV, msgRecievedKV, msg, payload, logger)
			if !recombined {
				return false
			}
			// non-triggering chunk: recombineChunks already acked and marked it recieved, nothing to upload
			if outputPath == "" {
				return true
			}

			uploadedVideoChunk := uploadVideoChunk(outputPath, baseStorageURL, msg, payload, logger)
			if !uploadedVideoChunk {
				return false
			}

			if terminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger) {
				CleanUpTempFolders(payload.JobID, logger)
				return true
			}

			return publishJetstreamCompleteMsg(js, msgRecievedKV, msg, payload, logger)
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

// recombine video chunks from nats msgs. Includes Updating the job status to be video-recombiner stage -> fetching chunks -> recombining all the chunks for a video
// returns a bool: false if any part fails and we want to stop or true if its done
func recombineChunks(
	nc handler.Publisher, jobMilestoneKV, msgRecievedKV jetstream.KeyValue, msg jetstream.Msg, payload handler.ChunkCompleteMessage, logger *slog.Logger,
) (bool, string) {
	ready, chunks, err := Add(msgRecievedKV, payload, logger)
	if err != nil {
		logger.Error("failed to record chunk", "job_id", payload.JobID, "chunk_index", payload.ChunkIndex, "err", err)
		sJetstream.NakWithErrHandling(logger, msg)
		return false, ""
	}

	// not the triggering chunk: nothing to combine yet, so this chunk's message is fully handled now
	if !ready {
		err = sJetstream.PutKeyKV(msgRecievedKV, fmt.Sprintf("%s.%d", payload.JobID, payload.ChunkIndex), []byte("processed"))
		if err != nil {
			logger.Error("failed to mark job chunk as recieved", "err", err)
			sJetstream.NakWithErrHandling(logger, msg)
			return false, ""
		}

		sJetstream.AckWithErrHandling(logger, msg)
		return true, ""
	}

	err = sJetstream.AdvanceMilestone(jobMilestoneKV, payload.JobID, sJetstream.MilestoneStatus{State: "PROCESSING", Stage: "video-recombiner"})
	if err != nil {
		logger.Error("failed to update job-milestones stage", "job_id", payload.JobID, "err", err)
		sJetstream.NakWithErrHandling(logger, msg)
		return false, ""
	}

	localChunks := make(map[int]string)
	var downloadErr error

	for idx, storageURL := range chunks {
		fileName := fmt.Sprintf("processed_chunk-%s", payload.JobID)

		localPath, err := storage.GetVideoChunk(storageURL, fileName)
		if err != nil {
			logger.Error("failed to download chunk", "job_id", payload.JobID, "chunk_index", idx, "err", err)
			downloadErr = err
			break
		}
		localChunks[idx] = localPath
	}
	if downloadErr != nil {
		CleanUpTempFolders(payload.JobID, logger)
		sJetstream.NakWithErrHandling(logger, msg)
		return false, ""
	}

	onProgress := handler.NewProgressReporter(nc, payload.JobID, "video-recombiner", logger)
	outputPath, err := CombineChunks(payload.JobID, localChunks, onProgress)
	if err != nil {
		logger.Error("failed to combine chunks", "job_id", payload.JobID, "err", err)
		CleanUpTempFolders(payload.JobID, logger)
		sJetstream.NakWithErrHandling(logger, msg)
		return false, ""
	}

	return true, outputPath
}

// terminate the nats msg if the job is cancelled and return true
func terminateIfCancelled(jobMilestoneKV jetstream.KeyValue, msg jetstream.Msg, jobID string, logger *slog.Logger) bool {
	isCancelled, err := sJetstream.IsJobCancelled(jobMilestoneKV, jobID)
	if err != nil {
		logger.Error("failed to check if job is cancelled", "job_id", jobID, "err", err)
		return true
	}
	if isCancelled {
		err := msg.Term()
		if err != nil {
			logger.Error("failed to terminate the cancelled jetstream msg", "job_id", jobID, "err", err)
		}
		return true
	}

	return false
}
