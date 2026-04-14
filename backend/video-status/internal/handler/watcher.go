package handler

import (
	"log/slog"
	"net/http"
	"time"
)

// need this because we are checking the next service
// from the current processing stage and used for the
// error msg
var nextService = map[string]string{
	"upload":         "scene-detector",
	"scene-detector": "transcoder",
	"transcoder":     "video-recombiner",
}

type ServiceURLs struct {
	SceneDetector string
	Transcoder    string
	Recombiner    string
}

func (s ServiceURLs) forStage(stage string) (string, bool) {
	next, ok := nextService[stage]
	if !ok {
		return "", false
	}

	urls := map[string]string{
		"scene-detector":   s.SceneDetector,
		"transcoder":       s.Transcoder,
		"video-recombiner": s.Recombiner,
	}

	url, ok := urls[next]
	if !ok || url == "" {
		return "", false
	}
	return url, true
}

func isServiceHealthy(baseURL string, logger *slog.Logger) bool {
	c := http.Client{Timeout: 3 * time.Second}

	resp, err := c.Get(baseURL + "/health")
	if err != nil {
		return false
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			logger.Error("error closing resp body", "err", err)
		}
	}()

	return resp.StatusCode == http.StatusOK
}
