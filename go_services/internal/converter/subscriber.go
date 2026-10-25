package converter

import (
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

const subSubject = "jobs.video.convert"

func ConsumeAndProcessVideo(
	baseStorageURL string, js jetstream.JetStream, processedKV, jobMilestoneKV jetstream.KeyValue, logger *slog.Logger,
) (jetstream.ConsumeContext, error) {
	return sJetstream.ConsumeDurable(js, sJetstream.ConsumerConfig{
		Subject:       subSubject,
		ServiceName:   "convert-worker",
		AckWait:       time.Hour, // Placeholder for now
		MaxAckPending: 1,
	}, func(msg jetstream.Msg) {
		payload, ok := sJetstream.UnmarshalJetstreamMsg[handler.VideoJobMessage](msg, logger)
		if !ok {
			return
		}

		processed, err := sJetstream.IsMsgProcessed(processedKV, payload.JobID)
		if err != nil {
			logger.Error("failed to check convert job processed", "err", err)
			return
		}
		if processed {
			logger.Debug("message already processed, skipping")
			sJetstream.AckWithErrHandling(logger, msg)
			return
		}

		if _, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
			return
		}

		defer cleanupTempUnprocessedDir(payload.JobID, logger)

		if err := sJetstream.AdvanceMilestone(jobMilestoneKV, payload.JobID, sJetstream.JobStatus{State: "PROCESSING", Stage: "format-converter"}); err != nil {
			logger.Error("failed to update job-milestones stage", "job_id", payload.JobID, "err", err)
			sJetstream.NakWithErrHandling(logger, msg)
			return
		}

		filePath, err := storage.DownloadVideo(payload.StorageURL, payload.JobID, 10*time.Minute)
		if err != nil {
			logger.Error("failed to download the unprocessed convert video", "job_id", payload.JobID, "err", err)
			sJetstream.NakWithErrHandling(logger, msg)
			return
		}

		outputPath, err := convertVideo(filePath, payload)
		if err != nil {
			logger.Error("failed to convert video to target format", "job_id", payload.JobID, "target_format", payload.TargetFormat, "err", err)
			sJetstream.NakWithErrHandling(logger, msg)
			return
		}

		if _, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
			return
		}

		fileName := filepath.Base(outputPath)
		destURL := fmt.Sprintf("%s/%s/%s/processed", baseStorageURL, payload.JobID, fileName)
		if _, err := storage.UploadProcessedVideo(destURL, outputPath, 10*time.Minute); err != nil {
			logger.Error("error saving format convert video to seaweedfs storage", "jobID", payload.JobID, "filePath", outputPath, "err", err)
			sJetstream.NakWithErrHandling(logger, msg)
			return
		}

		if _, stopJob := sJetstream.TerminateIfCancelled(jobMilestoneKV, msg, payload.JobID, logger); stopJob {
			return
		}

		if err := publishJetstreamProcessedMsg(js, processedKV, payload.JobID, logger); err != nil {
			sJetstream.NakWithErrHandling(logger, msg)
			return
		}

		sJetstream.AckWithErrHandling(logger, msg)
	})
}

func cleanupTempUnprocessedDir(jobID string, logger *slog.Logger) {
	if err := os.RemoveAll(storage.TempUnprocessedDir(jobID)); err != nil {
		logger.Warn("error removing the temp unprocessed folder", "jobID", jobID, "err", err)
	}
	if err := os.RemoveAll("/tmp/convert/" + jobID); err != nil {
		logger.Warn("error removed converted video folder", "jobID", jobID, "err", err)
	}
}
