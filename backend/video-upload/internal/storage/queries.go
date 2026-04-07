package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/google/uuid"
)

type postResult struct {
	JobID      string
	StorageURL string
}

// saves uploaded video to seedweedfs via POST request for downstream service use
func SaveUploadedVideo(videoFile multipart.File, storageUrl, fileName string) (postResult, error) {
	jobID := uuid.New().String()

	url := fmt.Sprintf("%s/%s/%s", storageUrl, jobID, fileName)
	req, err := http.NewRequest(http.MethodPut, url, videoFile)
	if err != nil {
		return postResult{}, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return postResult{}, fmt.Errorf("error connecting to seedweedfs: %w", err)
	}

	if resp.StatusCode >= 400 {
		return postResult{}, fmt.Errorf("seedweedfs returned status %d", resp.StatusCode)
	}

	return postResult{JobID: jobID, StorageURL: url}, nil
}

// fetch a completely processed video from seedweedfs storage
func GetProcessedVideo(storageUrl, jobID, fileName string) (io.ReadCloser, error) {
	resp, err := http.Get(fmt.Sprintf("%s/%s/%s/processed", storageUrl, jobID, fileName))
	if err != nil {
		return nil, fmt.Errorf("error connecting to seedweedfs, %w", err)
	}

	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("seedweedfs returned status %d", resp.StatusCode)
	}

	return resp.Body, nil
}
