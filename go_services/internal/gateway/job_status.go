package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/handler"
)

var osExit = os.Exit

type KVHandler struct {
	logger *slog.Logger
	kv     jetstream.KeyValue
}

func (h *KVHandler) getJobStatusKV(ctx context.Context, jobID string) (jetstream.KeyValueEntry, int, error) {
	entry, err := h.kv.Get(ctx, jobID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, http.StatusNotFound, errors.New("job not found")
		}
		h.logger.Error("failed to get job status from kv", "job_id", jobID, "err", err)
		return nil, http.StatusInternalServerError, errors.New("failed to get job status")
	}

	return entry, http.StatusOK, nil
}

func (h *KVHandler) updateJobStatusKV(ctx context.Context, JobID string, status JobStatus) error {
	data, err := json.Marshal(status)
	if err != nil {
		h.logger.Error("error marshalling status", "err", err)
		return err
	}

	_, err = h.kv.Put(ctx, JobID, data)
	if err != nil {
		h.logger.Error("failed to write job status to jobStatus kv", "job_id", JobID, "err", err)
		return err
	}

	return nil
}

type JobState string

const (
	StateProcessing JobState = "PROCESSING"
	StateComplete   JobState = "COMPLETE"
	StateFailed     JobState = "FAILED"
	StateDegraded   JobState = "DEGRADED"
)

type JobStatus struct {
	State    JobState `json:"state"`
	Stage    string   `json:"stage"`
	Progress *int     `json:"progress,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type jobStatusResponse struct {
	JobID    string   `json:"job_id"`
	State    JobState `json:"state"`
	Stage    string   `json:"stage"`
	Progress *int     `json:"progress,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type JobStatusHandler struct {
	Logger *slog.Logger
	NC     *nats.Conn
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

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(jobStatusResponse{JobID: jobID, State: status.State, Stage: status.Stage, Progress: status.Progress, Error: status.Error})
	if err != nil {
		j.Logger.Error("error encoding job status response", "err", err)
	}
}

type healthEvent struct {
	State JobState `json:"state"`
	Error string   `json:"error,omitempty"`
}

type healthProbeResult struct {
	stage  string
	state  JobState
	errMsg string
}

// streams a job's status over Server-Sent Events. KV.Watch delivers the current snapshot on connect 
// (for resuming/reload), then each milestone (new processign stage). Ephemeral progress ticks arrive 
// via pub/sub and a health probe scoped to the connection's current stage. Closes on terminal state
// (COMPLETE/FAILED) or client disconnect
func (j *JobStatusHandler) JobEvents(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		http.Error(w, "missing job_id", http.StatusBadRequest)
		j.Logger.Error("missing job_id path param")
		return
	}

	ctx := r.Context()

	watcher, err := j.KV.Watch(ctx, jobID)
	if err != nil {
		j.Logger.Error("failed to watch job status kv", "job_id", jobID, "err", err)
		http.Error(w, "failed to watch job status", http.StatusInternalServerError)
		return
	}
	defer func() {
		err := watcher.Stop()
		if err != nil {
			j.Logger.Error("failed to stop kv watcher", "job_id", jobID, "err", err)
		}
	}()

	progressCh := make(chan handler.ProgressMessage, 8)
	sub, err := j.NC.Subscribe("progress."+jobID, func(msg *nats.Msg) {
		var progressMsg handler.ProgressMessage
		err := json.Unmarshal(msg.Data, &progressMsg)
		if err != nil {
			return
		}

		select {
		case progressCh <- progressMsg:
		default: // reader is behind; progress ticks are best-effort (drop rather than block chan)
		}
	})
	if err != nil {
		j.Logger.Error("failed to subscribe to progress subject", "job_id", jobID, "err", err)
		http.Error(w, "failed to subscribe to job progress", http.StatusInternalServerError)
		return
	}
	defer func() {
		err := sub.Unsubscribe()
		if err != nil {
			j.Logger.Error("failed to unsubscribe from progress subject", "job_id", jobID, "err", err)
		}
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	err = rc.SetWriteDeadline(time.Time{})
	if err != nil {
		j.Logger.Debug("response writer does not support clearing write deadline", "err", err)
	}

	ticker := time.NewTicker(7 * time.Second)
	defer ticker.Stop()

	var current JobStatus
	var lastHealth JobState

	healthCh := make(chan healthProbeResult, 1)
	probing := false

	launchHealthProbe := func(stage string) {
		if probing {
			return
		}
		serviceURL, ok := j.URLs.forStage(stage)
		if !ok {
			return
		}
		probing = true
		go func() {
			state := StateProcessing
			errMsg := ""
			if !isServiceHealthy(serviceURL, j.Logger) {
				state = StateDegraded
				errMsg = fmt.Sprintf("service unavailable at stage: %s", stage)
			}
			healthCh <- healthProbeResult{stage: stage, state: state, errMsg: errMsg}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return

		case entry, ok := <-watcher.Updates():
			if !ok {
				return
			}
			if entry == nil {
				continue
			}
			err := json.Unmarshal(entry.Value(), &current)
			if err != nil {
				j.Logger.Error("failed to unmarshal job status", "job_id", jobID, "err", err)
				continue
			}

			resp := jobStatusResponse{JobID: jobID, State: current.State, Stage: current.Stage, Progress: current.Progress, Error: current.Error}
			err = writeSSEEvent(w, "status", resp)
			if err != nil {
				return
			}
			err = rc.Flush()
			if err != nil {
				j.Logger.Error("failed to flush SSE event to user", "job_id", jobID, "err", err)
				return
			}

			if current.State == StateComplete || current.State == StateFailed {
				return
			}
			launchHealthProbe(current.Stage)

		case progressMsg := <-progressCh:
			err := writeSSEEvent(w, "progress", progressMsg)
			if err != nil {
				return
			}
			err = rc.Flush()
			if err != nil {
				j.Logger.Error("failed to flush SSE event to user", "job_id", jobID, "err", err)
				return
			}

		case <-ticker.C:
			launchHealthProbe(current.Stage)

		case result := <-healthCh:
			probing = false
			if result.stage != current.Stage || result.state == lastHealth {
				continue // stale (stage moved on while probing) or unchanged
			}
			lastHealth = result.state
			err := writeSSEEvent(w, "health", healthEvent{State: result.state, Error: result.errMsg})
			if err != nil {
				return
			}
			err = rc.Flush()
			if err != nil {
				j.Logger.Error("failed to flush SSE event to user", "job_id", jobID, "err", err)
				return
			}
		}
	}
}
