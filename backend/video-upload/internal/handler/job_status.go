package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"video-upload/internal/service"
)

type jobStatusResponse struct {
	JobID string `json:"job_id"`
	State string `json:"state"`
}

type JobStatusHandler struct {
	Logger  *slog.Logger
	Tracker *service.CompletedJobs
}

func (j *JobStatusHandler) PollJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		http.Error(w, "missing job_id", http.StatusBadRequest)
		j.Logger.Error("missing job_id path param")
		return
	}

	state := "PROCESSING"
	if j.Tracker.IsDone(jobID) {
		state = "COMPLETE"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobStatusResponse{JobID: jobID, State: state})
}
