package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/nats-io/nats.go/jetstream"
)

type JobState string

const (
	StateProcessing JobState = "PROCESSING"
	StateComplete   JobState = "COMPLETE"
	StateFailed     JobState = "FAILED"
	StateDegraded   JobState = "DEGRADED"
)

type JobStatus struct {
	State JobState `json:"state"`
	Stage string   `json:"stage"`
	Error string   `json:"error,omitempty"`
}

type jobStatusResponse struct {
	JobID string   `json:"job_id"`
	State JobState `json:"state"`
	Stage string   `json:"stage"`
	Error string   `json:"error,omitempty"`
}

type JobStatusHandler struct {
	Logger *slog.Logger
	KV     jetstream.KeyValue
	URLs   ServiceURLs
}

func (j *JobStatusHandler) PollJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		http.Error(w, "missing job_id", http.StatusBadRequest)
		j.Logger.Error("missing job_id path param")
		return
	}

	kh := KVHandler{logger: j.Logger, kv: j.KV}

	entry, httpStatusCode, err := kh.getJobStatusKV(r.Context(), jobID)
	if err != nil {
		http.Error(w, err.Error(), httpStatusCode)
		return
	}

	var status JobStatus
	err = json.Unmarshal(entry.Value(), &status)
	if err != nil {
		j.Logger.Error("failed to unmarshal job status", "job_id", jobID, "err", err)
		http.Error(w, "failed to parse job status", http.StatusInternalServerError)
		return
	}

	if status.State == StateProcessing || status.State == StateDegraded {
		status = checkServiceHealth(status, j.URLs, kh.logger)
		err := kh.updateJobStatusKV(r.Context(), jobID, status)
		if err != nil {
			j.Logger.Error("failed to update job status KV", "job_id", jobID, "err", err)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(jobStatusResponse{JobID: jobID, State: status.State, Stage: status.Stage, Error: status.Error})
	if err != nil {
		j.Logger.Error("error encoding job status response", "err", err)
	}
}

func checkServiceHealth(status JobStatus, urls ServiceURLs, logger *slog.Logger) JobStatus {
	serviceURL, ok := urls.forStage(status.Stage)
	if !ok {
		return status
	}

	if isServiceHealthy(serviceURL, logger) {
		status.State = StateProcessing
		status.Error = ""
	} else {
		status.State = StateDegraded
		status.Error = fmt.Sprintf("service unavailable at stage: %s", nextService[status.Stage])
	}

	return status
}
