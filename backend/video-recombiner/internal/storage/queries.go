package storage

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// upload the combined video from all its chunks back to seaweedfs storage
func UploadRecombinedVideo(baseStorageURL, filePath, jobID string) (string, error) {
	fileName := filepath.Base(filePath)
	url := fmt.Sprintf("%s/%s/%s/processed", baseStorageURL, jobID, fileName)

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error opening transcoded video chunk file: %w", err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			log.Printf("error closing the file: %v", err)
		}
	}()

	req, err := http.NewRequest(http.MethodPut, url, file)
	if err != nil {
		return "", fmt.Errorf("error creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error connecting to seaweedfs storage: %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Printf("error closing the response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("seaweedfs upload failed with status: %d", resp.StatusCode)
	}

	return url, err
}

var removeAll = os.RemoveAll

// fetch the processed video chunk from seaweedfs storage
func GetProcessedVideoChunk(storageURL, jobID string) (string, error) {
	resp, err := http.Get(storageURL)
	if err != nil {
		return "", fmt.Errorf("error connecting to seedweedfs, %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Printf("error closing the response body, %v", err)
		}
	}()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return "", fmt.Errorf("video not found")
	case http.StatusForbidden:
		return "", fmt.Errorf("access denied")
	case http.StatusInternalServerError:
		return "", fmt.Errorf("error accessing seedweedfs")
	}

	trimmed := strings.TrimSuffix(storageURL, "/processed")
	filename := trimmed[strings.LastIndex(trimmed, "/")+1:]
	jobDir := filepath.Join("/tmp/processed_chunk-" + jobID)

	err = os.MkdirAll(jobDir, 0755)
	if err != nil {
		return "", fmt.Errorf("error created job temp dir: %w", err)
	}

	filePath := filepath.Join(jobDir, filename)
	outFile, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("error creating video file: %w", err)
	}
	defer func() {
		err := outFile.Close()
		if err != nil {
			log.Printf("error closing the file, %v", err)
		}
	}()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		err := removeAll(jobDir)
		if err != nil {
			return "", fmt.Errorf("error removing all files: %w", err)
		}
		return "", fmt.Errorf("error writing video to file: %w", err)
	}

	return filePath, nil
}
