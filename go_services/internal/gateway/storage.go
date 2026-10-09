package gateway

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var storageClient = &http.Client{
	Transport: &http.Transport{ResponseHeaderTimeout: 10 * time.Second},
}

// fetch a completely processed video from seedweedfs storage
func GetProcessedVideo(storageUrl, jobID, fileName string) (io.ReadCloser, error) {
	resp, err := storageClient.Get(fmt.Sprintf("%s/%s/%s/processed", storageUrl, jobID, fileName))
	if err != nil {
		return nil, fmt.Errorf("error connecting to seedweedfs, %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if err := resp.Body.Close(); err != nil {
			return nil, errors.New("error closing resp body")
		}
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, errors.New("video not found")
	case http.StatusForbidden:
		return nil, errors.New("access denied")
	case http.StatusInternalServerError:
		return nil, errors.New("error accessing seedweedfs")
	}

	return resp.Body, nil
}
