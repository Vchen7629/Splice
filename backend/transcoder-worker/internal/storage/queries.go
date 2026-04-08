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

// save the video chunk transcoded to a target resolution back onto seaweedfs storage
func SaveTranscodedVideoChunk(baseStorageURL, filePath, jobID string) (string, error) {
	fileName := filepath.Base(filePath)
	url := fmt.Sprintf("%s/%s/processed/%s", baseStorageURL, jobID, fileName)

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("error opening transcoded video file: %w", err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			log.Printf("error closing the file, %v", err)
		}
	}()

	req, err := http.NewRequest(http.MethodPut, url, file)
	if err != nil {
		return "", fmt.Errorf("error creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error connecting to seaweedfs: %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Printf("error closing the response body, %v", err)
		}
	}()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("seaweedfs upload failed with status: %d", resp.StatusCode)
	}

	return url, nil
}

var removeAll = os.RemoveAll

// fetch the unprocessed video chunk seaweedfs storage
func GetUnprocessedVideoChunk(storageURL, jobID string) (string, error) {
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
