package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"video-upload/internal/service"

	"github.com/nats-io/nats.go/jetstream"
)

type VideoHandler struct {
	Logger         *slog.Logger
	JS             jetstream.JetStream
	OutputDir      string
	MaxUploadBytes int64
}

type uploadResponse struct {
	JobID string `json:"job_id"`
}

// handler for video upload POST requests, Accepts a multipart video upload, saves it to disk,
// and publishes a scene split message to NATS for downstream processing
func (v *VideoHandler) UploadVideo(w http.ResponseWriter, r *http.Request) {
	limit := v.MaxUploadBytes
	if limit == 0 {
		limit = 10 << 30 // 10 GB
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		v.Logger.Error("invalid request multipart form", "err", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		http.Error(w, "missing video field", http.StatusBadRequest)
		v.Logger.Error("missing video file in request")
		return
	}
	defer func() {
		err := file.Close()
		if err != nil {
			http.Error(w, "error closing open file", http.StatusBadRequest)
			v.Logger.Error("error closing open file", "err", err)
			return
		}
	}()

	targetRes := r.FormValue("target_resolution")
	if targetRes == "" {
		http.Error(w, "missing target_resolution field", http.StatusBadRequest)
		v.Logger.Error("missing target_resolution field")
		return
	}

	result, err := service.SaveUploadedVideo(file, v.OutputDir, header.Filename, v.Logger)
	if err != nil {
		http.Error(w, "failed to save uploaded video", http.StatusInternalServerError)
		v.Logger.Error("failed to save uploaded video", "err", err)
		return
	}

	err = PublishVideoMetadata(
		v.JS, service.SceneSplitMessage{
			JobID: result.JobID, TargetResolution: targetRes, StoragePath: result.StoragePath,
		},
	)
	if err != nil {
		http.Error(w, "unable to send process request msg to system", http.StatusInternalServerError)
		v.Logger.Error("error publishing split video request to nats", "err", err)
		return
	}

	v.Logger.Info("video upload job submitted", "job_id", result.JobID, "file", header.Filename)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(uploadResponse{JobID: result.JobID})
	if err != nil {
		http.Error(w, "error encoding http response", http.StatusInternalServerError)
		v.Logger.Error("error encoding success http response", "err", err)
		return
	}
}

// handler for streaming the completed out video for a given job ID
func (v *VideoHandler) DownloadVideo(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		http.Error(w, "missing job_id", http.StatusBadRequest)
		v.Logger.Error("missing job_id path param")
		return
	}

	outputPath := filepath.Join(v.OutputDir, "jobs", jobID, "output.mp4")
	v.Logger.Debug("serving output video", "job_id", jobID, "path", outputPath)
	http.ServeFile(w, r, outputPath)
}
