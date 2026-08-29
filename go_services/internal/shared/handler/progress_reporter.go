package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

type Publisher interface {
	Publish(subj string, data []byte) error
}

// returns a throttled progress callback that publishs ProgressMessage JSON
// to progress.{jobID}, skipping duplicate integer percents
func NewProgressReporter(pub Publisher, jobID, stage string, logger *slog.Logger) func(pct int) {
	lastProgress := -1

	return func(pct int) {
		if pct == lastProgress {
			return
		}
		lastProgress = pct

		data, err := json.Marshal(ProgressMessage{
			JobID:    jobID,
			Stage:    stage,
			Progress: pct,
		})
		if err != nil {
			logger.Error("failed to marshal progress message", "job_id", jobID, "stage", stage, "err", err)
			return
		}

		err = pub.Publish(fmt.Sprintf("progress.%s", jobID), data)
		if err != nil {
			logger.Error("failed to publish progress message", "job_id", jobID, "stage", stage, "err", err)
			return
		}
	}
}
