package chunkworker

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	transcodeCount   *prometheus.CounterVec
	transcodeLatency prometheus.Histogram

	denoiseCount   *prometheus.CounterVec
	denoiseLatency prometheus.Histogram
}

// registers the custom prometheus metrics
func NewMetrics(reg prometheus.Registerer) *metrics {
	return &metrics{
		transcodeCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "chunk_worker_transcode_count",
			Help: "Transcode jobs finished",
		}, []string{"status"}), // success|failure
		transcodeLatency: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Name:    "chunk_worker_transcode_duration_seconds",
			Unit:    "seconds",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
			Help:    "seconds it takes to transcode one video chunk",
		}),
		denoiseCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "chunk_worker_denoise_count",
			Help: "Denoise jobs finished",
		}, []string{"status"}), // success|failure
		denoiseLatency: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Name:    "chunk_worker_denoise_duration_seconds",
			Unit:    "seconds",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
			Help:    "seconds it takes to denoise one video chunk",
		}),
	}
}
