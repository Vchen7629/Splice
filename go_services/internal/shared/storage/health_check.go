package storage

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// send an http request to shared to storage to see if its reachable
func CheckHealth(storageURL string, logger *slog.Logger, timeout time.Duration) error {
	httpClient := &http.Client{Timeout: timeout}
	resp, err := httpClient.Get(storageURL + "/")

	if err != nil {
		return fmt.Errorf("error connecting to seedweedfs: %w", err)
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			logger.Warn("failed to close resp body for check health", "err", err)
		}
	}()

	// an empty filer can return 404 on "/", any non-5xx response means it's reachable
	if resp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("seedweedfs returned status %d", resp.StatusCode)
	}

	return nil
}
