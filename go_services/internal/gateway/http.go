package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"splice.com/go_services/internal/shared/handler"
	sJetstream "splice.com/go_services/internal/shared/jetstream"
	"splice.com/go_services/internal/shared/middleware"

	"github.com/go-playground/validator/v10"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func StartHttpApi(
	logger *slog.Logger,
	nc *nats.Conn,
	js jetstream.JetStream,
	kv jetstream.KeyValue,
	httpPort, storageURL string,
	urls ServiceURLs,
) *http.Server {
	router := http.NewServeMux()

	vh := &videoHandler{logger: logger, js: js, kv: kv, storageURL: storageURL}
	jh := &JobStatusHandler{Logger: logger, NC: nc, KV: kv, URLs: urls}

	router.HandleFunc("POST /jobs/upload", vh.uploadVideoRoute)
	router.HandleFunc("POST /jobs/download", vh.downloadVideoRoute)
	router.HandleFunc("GET /jobs/{id}/status", jh.PollJobStatus)
	router.HandleFunc("GET /jobs/{id}/events", jh.JobEvents)

	server := &http.Server{
		Addr:              ":" + httpPort,
		Handler:           Cors(middleware.ApiRequestLogging(router)),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadTimeout:       15 * time.Minute,
		WriteTimeout:      15 * time.Minute,
	}

	go func() {
		fmt.Printf("server running on http://localhost:%s\n", httpPort)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "err", err)
			osExit(1)
		}
	}()

	return server
}

type videoHandler struct {
	logger         *slog.Logger
	js             jetstream.JetStream
	kv             jetstream.KeyValue
	storageURL     string
	maxUploadBytes int64
}

type uploadResponse struct {
	JobID string `json:"job_id"`
}

// handler for video upload POST requests, Accepts a multipart video upload, saves it to disk,
// and publishes a scene split message to NATS for downstream processing
func (v *videoHandler) uploadVideoRoute(w http.ResponseWriter, r *http.Request) {
	limit := v.maxUploadBytes
	if limit == 0 {
		limit = 10 << 30 // 10 GB
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		v.logger.Error("invalid request multipart form", "err", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		http.Error(w, "missing video field", http.StatusBadRequest)
		v.logger.Error("missing video file in request")
		return
	}
	defer func() {
		err := file.Close()
		if err != nil {
			http.Error(w, "error closing open file", http.StatusBadRequest)
			v.logger.Error("error closing open file", "err", err)
			return
		}
	}()

	targetRes := r.FormValue("target_resolution")
	if targetRes == "" {
		http.Error(w, "missing target_resolution field", http.StatusBadRequest)
		v.logger.Error("missing target_resolution field")
		return
	}

	sourceRes := r.FormValue("source_resolution")
	if sourceRes == "" {
		http.Error(w, "missing source_resolution field", http.StatusBadRequest)
		v.logger.Error("missing source_resolution field")
		return
	}

	processType := r.FormValue("process_type")
	if processType == "" {
		http.Error(w, "missing process_type field", http.StatusBadRequest)
		v.logger.Error("missing process_type field")
		return
	}

	var pubSubject string

	switch processType {
	case "Transcode":
		pubSubject = "jobs.video.scene-split"
	case "Upscale":
		pubSubject = "jobs.video.upscale"
	case "Denoise":
		pubSubject = "jobs.video.denoise"
	case "Convert":
		pubSubject = "jobs.video.convert"
	default:
		http.Error(w, "invalid process_type field", http.StatusBadRequest)
		v.logger.Error("invalid process_type field", "process_type", processType)
		return
	}

	v.logger.Debug("pubsubject called", "subject", pubSubject)

	result, err := SaveUploadedVideo(file, v.storageURL, header.Filename)
	if err != nil {
		http.Error(w, "failed to save uploaded video", http.StatusInternalServerError)
		v.logger.Error("failed to save uploaded video", "err", err)
		return
	}

	v.logger.Debug("pubSubject is", "pubSubject", pubSubject)

	kh := KVHandler{logger: v.logger, kv: v.kv}
	err = kh.updateJobStatusKV(r.Context(), result.JobID, JobStatus{State: StateProcessing, Stage: "upload"})
	if err != nil {
		http.Error(w, "failed to record job status", http.StatusInternalServerError)
		return
	}

	err = sJetstream.PublishJetstreamMsg(
		v.js, handler.VideoJobMessage{
			JobID: result.JobID, TargetResolution: targetRes, SourceResolution: sourceRes, StorageURL: result.StorageURL,
		}, pubSubject,
	)
	if err != nil {
		v.logger.Error("error publishing request to nats", "err", err)
		kvErr := kh.updateJobStatusKV(r.Context(), result.JobID, JobStatus{State: StateFailed, Stage: "upload"})
		if kvErr != nil {
			v.logger.Error("failed to mark job failed after publish error", "err", kvErr)
		}
		http.Error(w, "unable to send process request msg to system", http.StatusInternalServerError)
		return
	}

	v.logger.Info("video upload job submitted", "job_id", result.JobID, "file", header.Filename)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(uploadResponse{JobID: result.JobID})
	if err != nil {
		http.Error(w, "error encoding http response", http.StatusInternalServerError)
		v.logger.Error("error encoding success http response", "err", err)
		return
	}
}

// handler for streaming the completed out video for a given job ID
func (v *videoHandler) downloadVideoRoute(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		JobID    string `json:"job_id" validate:"required,min=2"`
		FileName string `json:"file_name" validate:"required,min=2"`
	}

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		v.logger.Error("error decoding the request body", "err", err)
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}

	err = validator.New().Struct(payload)
	if err != nil {
		v.logger.Error("error validating request body", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, err := GetProcessedVideo(v.storageURL, payload.JobID, payload.FileName)
	if err != nil {
		v.logger.Error("failed to fetch processed video", "err", err)
		http.Error(w, "failed to fetch video", http.StatusInternalServerError)
		return
	}
	defer func() {
		err := body.Close()
		if err != nil {
			v.logger.Warn("failed to close response body for Get Processed Video", "err", err)
		}
	}()

	v.logger.Debug("fetching output video", "job_id", payload.JobID, "fileName", payload.FileName)

	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(payload.FileName))
	w.Header().Set("Content-Type", "application/octet-stream")

	_, err = io.Copy(w, body)
	if err != nil {
		v.logger.Error("error streaming video to response", "err", err)
		return
	}
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
