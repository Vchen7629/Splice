package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type UploadResult struct {
	JobID       string
	StoragePath string
}

// Saves the uploaded video from frontend to local storage, returns jobID and path
func SaveUploadedVideo(src io.Reader, storageDir, filename string) (UploadResult, error) {
	jobID := uuid.New().String()

	jobDir := filepath.Join(storageDir, "jobs", jobID)

	err := os.MkdirAll(jobDir, 0755)
	if err != nil {
		return UploadResult{}, fmt.Errorf("create job dir err: %w", err)
	}

	destPath := filepath.Join(jobDir, filepath.Base(filename))

	f, err := os.Create(destPath)
	if err != nil {
		return UploadResult{}, fmt.Errorf("create video file error: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, src)
	if err != nil {
		return UploadResult{}, fmt.Errorf("write video error: %w", err)
	}

	return UploadResult{JobID: jobID, StoragePath: destPath}, nil
}
