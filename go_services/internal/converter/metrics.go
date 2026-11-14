package converter

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	convertCount   *prometheus.CounterVec
	convertLatency prometheus.Histogram
}

// registers the custom convert prometheus metrics
func NewMetrics(reg prometheus.Registerer) *metrics {
	return &metrics{
		convertCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "converter_convert_count",
			Help: "Convert jobs finished",
		}, []string{"status"}), // success|failure
		convertLatency: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Name:    "converter_convert_duration_seconds",
			Unit:    "seconds",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
			Help:    "seconds it takes to convert one video to another format",
		}),
	}
}
