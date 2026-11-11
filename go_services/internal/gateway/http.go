package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"splice.com/go_services/internal/shared/pipeline"
	"splice.com/go_services/internal/shared/service"
)

type Config struct {
	HTTPPort             string
	MaxConcurrentUploads int
	StorageURL           string
	URLs                 ServiceURLs
}

type videoHandler struct {
	logger         *slog.Logger
	js             jetstream.JetStream
	kv             jetstream.KeyValue
	storageURL     string
	uploadSem      chan struct{}
	maxUploadBytes int64
}

type JobStatusHandler struct {
	Logger       *slog.Logger
	NC           *nats.Conn
	KV           jetstream.KeyValue
	URLs         ServiceURLs
	HealthClient *http.Client
}

type cancelHandler struct {
	logger *slog.Logger
	kv     jetstream.KeyValue
}

func StartHttpApi(
	logger *slog.Logger,
	nc *nats.Conn,
	js jetstream.JetStream,
	kv jetstream.KeyValue,
	cfg Config,
) *http.Server {
	router := http.NewServeMux()

	vh := &videoHandler{logger: logger, js: js, kv: kv, uploadSem: make(chan struct{}, cfg.MaxConcurrentUploads), storageURL: cfg.StorageURL}
	jh := &JobStatusHandler{Logger: logger, NC: nc, KV: kv, URLs: cfg.URLs, HealthClient: &http.Client{Timeout: 3 * time.Second}}
	ch := &cancelHandler{logger: logger, kv: kv}

	router.HandleFunc("POST /jobs/upload", vh.uploadVideoRoute)
	router.HandleFunc("POST /jobs/download", vh.downloadVideoRoute)

	router.HandleFunc("GET /jobs/{id}/status", jh.PollJobStatus)
	router.HandleFunc("GET /jobs/{id}/events", jh.JobEvents)
	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"status": "ok"}`))
		if err != nil {
			logger.Error("failed to write the http response json", "err", err)
		}
	})

	router.HandleFunc("DELETE /jobs/{id}", ch.cancelProcessingRoute)

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           Cors(service.ApiRequestLogging(router)),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadTimeout:       15 * time.Minute,
		WriteTimeout:      15 * time.Minute,
	}

	service.ListenAndServeAsync(server, logger)

	return server
}

// marshalls payload and writes it as one names SSE event
func writeSSEEvent(w io.Writer, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}

func (c *cancelHandler) cancelProcessingRoute(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if err := validateJobID(jobID); err != nil {
		c.logger.Error(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	w.Header().Set("Content-Type", "application/json")

	var result pipeline.JobStatus
	var outcome milestoneWriteOutcome

	result, outcome, err := tryUpdateMilestone(ctx, c.kv, jobID, pipeline.JobStatus{State: pipeline.StateCancelled})
	if err != nil {
		errMsg := "failed to update current stage for jobID as CANCELLED"
		http.Error(w, errMsg, http.StatusInternalServerError)
		c.logger.Error(errMsg, "job_id", jobID, "err", err)
		return
	}
	if outcome == milestoneNotFound {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	err = json.NewEncoder(w).Encode(jobStatusResponse{JobID: jobID, State: result.State, Stage: result.Stage})
	if err != nil {
		c.logger.Error("error encoding success http response", "err", err)
		return
	}
}
