package recombiner

import (
	"fmt"
	"log/slog"
	"os"

	"splice.com/go_services/internal/shared/storage"
)

var removeAll = os.RemoveAll

// Remove the tmp folders for the jobID after processing is done
func CleanUpTempFolders(jobID string, logger *slog.Logger) {
	err := removeAll(storage.TempUnprocessedDir(fmt.Sprintf("processed_chunk-%s", jobID)))
	if err != nil {
		logger.Warn("failed to clean up chunk temp dir", "job_id", jobID, "err", err)
	}

	err = removeAll("/tmp/jobs/" + jobID)
	if err != nil {
		logger.Warn("failed to clean up job temp dir", "job_id", jobID, "err", err)
	}
}
