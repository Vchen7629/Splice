package service

import (
	"log/slog"
	"os"
)

var removeAll = os.RemoveAll

// Remove the tmp folders for the jobID after processing is done
func CleanUpTempFolders(jobID string, logger *slog.Logger) {
	err := removeAll("/tmp/processed_chunk-" + jobID)
	if err != nil {
		logger.Warn("failed to clean up chunk temp dir", "job_id", jobID, "err", err)
	}

	err = removeAll("/tmp/jobs/" + jobID)
	if err != nil {
		logger.Warn("failed to clean up job temp dir", "job_id", jobID, "err", err)
	}
}
