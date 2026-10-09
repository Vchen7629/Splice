package gateway

import (
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
		resp.Body.Close()
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, fmt.Errorf("video not found")
	case http.StatusForbidden:
		return nil, fmt.Errorf("access denied")
	case http.StatusInternalServerError:
		return nil, fmt.Errorf("error accessing seedweedfs")
	}

	return resp.Body, nil
}
