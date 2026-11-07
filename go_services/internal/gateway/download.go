package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-playground/validator/v10"
	"splice.com/go_services/internal/shared/storage"
)

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

	// validation for security
	if err := storage.ValidatePathSegment(payload.FileName); err != nil {
		http.Error(w, "invalid video filename", http.StatusBadRequest)
		return
	}
	if err := validateJobID(payload.JobID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body, err := getProcessedVideo(r.Context(), v.storageURL, payload.JobID, payload.FileName)
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

	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(payload.FileName)}))
	w.Header().Set("Content-Type", "application/octet-stream")

	if _, err := io.Copy(w, body); err != nil {
		v.logger.Error("error streaming video to response", "err", err)
		panic(http.ErrAbortHandler)
	}
}

var storageClient = func() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 10 * time.Second
	return &http.Client{Transport: t, Timeout: 30 * time.Minute}
}()

// fetch a completely processed video from seedweedfs storage
func getProcessedVideo(ctx context.Context, storageUrl, jobID, fileName string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s/%s/processed", storageUrl, jobID, fileName), nil)
	if err != nil {
		return nil, fmt.Errorf("error building storage request: %w", err)
	}

	resp, err := storageClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error connecting to seedweedfs: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, errors.New("video not found")
		case http.StatusForbidden:
			return nil, errors.New("access denied")
		case http.StatusInternalServerError:
			return nil, errors.New("error accessing seedweedfs")
		default:
			return nil, fmt.Errorf("unexpected storage status: %d", resp.StatusCode)
		}
	}

	return resp.Body, nil
}
