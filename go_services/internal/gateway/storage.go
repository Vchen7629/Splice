package gateway

import (
	"fmt"
	"io"
	"net/http"
)

// fetch a completely processed video from seedweedfs storage
func GetProcessedVideo(storageUrl, jobID, fileName string) (io.ReadCloser, error) {
	resp, err := http.Get(fmt.Sprintf("%s/%s/%s/processed", storageUrl, jobID, fileName))
	if err != nil {
		return nil, fmt.Errorf("error connecting to seedweedfs, %w", err)
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
