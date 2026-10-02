package gateway

import (
	"log/slog"
	"net/http"
)

type ServiceURLs struct {
	SceneDetector  string
	Transcoder     string
	Recombiner     string
	VideoUpscaling string
}

func (s ServiceURLs) forStage(stage string) (string, bool) {
	urls := map[string]string{
		"scene-detector":   s.SceneDetector,
		"transcoder":       s.Transcoder,
		"video-recombiner": s.Recombiner,
		"video-upscaling":  s.VideoUpscaling,
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
