package gateway

import (
	"log/slog"
	"net/http"
)

type ServiceURLs struct {
	SceneDetector   string
	ChunkWorker     string
	Recombiner      string
	VideoUpscaling  string
	FormatConverter string
}

func (s ServiceURLs) forStage(stage string) (string, bool) {
	urls := map[string]string{
		"scene-detector":   s.SceneDetector,
		"chunk-worker":     s.ChunkWorker,
		"video-recombiner": s.Recombiner,
		"video-upscaling":  s.VideoUpscaling,
		"format-converter": s.FormatConverter,
	}

	url, ok := urls[stage]
	if !ok || url == "" {
		return "", false
	}
	return url, true
}

func isServiceHealthy(httpClient *http.Client, baseURL string, logger *slog.Logger) bool {
	resp, err := httpClient.Get(baseURL + "/health")
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
