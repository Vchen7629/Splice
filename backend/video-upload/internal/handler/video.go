package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"video-upload/internal/service"
	"video-upload/internal/storage"

	"github.com/go-playground/validator/v10"
	"github.com/nats-io/nats.go/jetstream"
)

type VideoHandler struct {
	Logger         *slog.Logger
	JS             jetstream.JetStream
	StorageURL     string
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

	result, err := storage.SaveUploadedVideo(file, v.StorageURL, header.Filename)
	if err != nil {
		http.Error(w, "failed to save uploaded video", http.StatusInternalServerError)
		v.Logger.Error("failed to save uploaded video", "err", err)
		return
	}

	err = PublishVideoMetadata(
		v.JS, service.SceneSplitMessage{
			JobID: result.JobID, TargetResolution: targetRes, StorageURL: result.StorageURL,
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
	var payload struct {
		JobID    string `json:"job_id" validate:"required,min=2"`
		FileName string `json:"file_name" validate:"required,min=2"`
	}

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		v.Logger.Error("error decoding the request body", "err", err)
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}

	err = validator.New().Struct(payload)
	if err != nil {
		v.Logger.Error("error validating request body", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, err := storage.GetProcessedVideo(v.StorageURL, payload.JobID, payload.FileName)
	if err != nil {
		v.Logger.Error("failed to fetch processed video", "err", err)
		http.Error(w, "failed to fetch video", http.StatusInternalServerError)
		return
	}
	defer func() {
		err := body.Close()
		if err != nil {
			v.Logger.Warn("failed to close response body for Get Processed Video", "err", err)
		}
	}()

	v.Logger.Debug("fetching output video", "job_id", payload.JobID, "fileName", payload.FileName)

	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(payload.FileName))
	w.Header().Set("Content-Type", "application/octet-stream")

	_, err = io.Copy(w, body)
	if err != nil {
		v.Logger.Error("error streaming video to response", "err", err)
		return
	}
}
