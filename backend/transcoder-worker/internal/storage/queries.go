package storage

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// save the video chunk transcoded to a target resolution back onto seaweedfs storage
func SaveTranscodedVideoChunk(baseStorageURL, filePath, jobID string) (string, error) {
	fileName := filepath.Base(filePath)
	url := fmt.Sprintf("%s/%s/processed/%s", baseStorageURL, jobID, fileName)

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error opening transcoded video file: %w", err)
	}
	defer file.Close()

	req, err := http.NewRequest(http.MethodPut, url, file)
	if err != nil {
		return "", fmt.Errorf("error creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error connecting to seaweedfs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("seaweedfs upload failed with status: %d", resp.StatusCode)
	}

	return url, nil
}

// fetch the unprocessed video chunk seaweedfs storage
func GetUnprocessedVideoChunk(storageURL, jobID string) (string, error) {
	resp, err := http.Get(storageURL)
	if err != nil {
		return "", fmt.Errorf("error connecting to seedweedfs, %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return "", fmt.Errorf("video not found")
	case http.StatusForbidden:
		return "", fmt.Errorf("access denied")
	case http.StatusInternalServerError:
		return "", fmt.Errorf("error accessing seedweedfs")
	}

	filename := storageURL[strings.LastIndex(storageURL, "/")+1:]
	jobDir := filepath.Join("/tmp/temp-unprocessed-" + jobID)

	err = os.MkdirAll(jobDir, 0755)
	if err != nil {
		return "", fmt.Errorf("error created job temp dir: %w", err)
	}

	filePath := filepath.Join(jobDir, filename)
	outFile, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("error creating video file: %w", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		os.RemoveAll(jobDir)
		return "", fmt.Errorf("error writing video to file: %w", err)
	}

	return filePath, nil
}
