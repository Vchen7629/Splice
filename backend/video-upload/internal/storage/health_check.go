package storage

import (
	"fmt"
	"net/http"
)

// send an http request to shared to storage to see if its reachable
func CheckHealth(storageURL string) error {
	resp, err := http.Get(storageURL + "/dir/status")

	if err != nil {
		return fmt.Errorf("error connecting to seedweedfs: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("seedweedfs returned status %d", resp.StatusCode)
	}

	return nil
}
