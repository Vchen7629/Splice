package storage

import (
	"fmt"
	"log/slog"
	"net/http"
)

// send an http request to shared to storage to see if its reachable
func CheckHealth(storageURL string, logger *slog.Logger) error {
	resp, err := http.Get(storageURL + "/dir/status")

	if err != nil {
		return fmt.Errorf("error connecting to seedweedfs: %w", err)
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			logger.Warn("failed to close resp body for check health", "err", err)
		}
	}()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("seedweedfs returned status %d", resp.StatusCode)
	}

	return nil
}
