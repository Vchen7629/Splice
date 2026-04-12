package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/nats-io/nats.go/jetstream"
)

type JobState string

const (
	StateProcessing JobState = "PROCESSING"
	StateComplete   JobState = "COMPLETE"
	StateFailed     JobState = "FAILED"
)

type JobStatus struct {
	State JobState `json:"state"`
	Error string   `json:"error,omitempty"`
}

type jobStatusResponse struct {
	JobID string   `json:"job_id"`
	State JobState `json:"state"`
	Error string   `json:"error,omitempty"`
}

type JobStatusHandler struct {
	Logger *slog.Logger
	KV     jetstream.KeyValue
}

func (j *JobStatusHandler) PollJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		http.Error(w, "missing job_id", http.StatusBadRequest)
		j.Logger.Error("missing job_id path param")
		return
	}

	entry, err := j.KV.Get(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		j.Logger.Error("failed to get job status from kv", "job_id", jobID, "err", err)
		http.Error(w, "failed to get job status", http.StatusInternalServerError)
		return
	}

	var status JobStatus
	err = json.Unmarshal(entry.Value(), &status)
	if err != nil {
		j.Logger.Error("failed to unmarshal job status", "job_id", jobID, "err", err)
		http.Error(w, "failed to parse job status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(jobStatusResponse{JobID: jobID, State: status.State, Error: status.Error})
	if err != nil {
		j.Logger.Error("error encoding job status response", "err", err)
	}
}
