package recombiner

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	recombineCount   *prometheus.CounterVec
	recombineLatency prometheus.Histogram
}

// registers the custom prometheus metrics
func NewMetrics(reg prometheus.Registerer) *metrics {
	return &metrics{
		recombineCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "recombiner_recombine_count",
			Help: "Recombine jobs finished",
		}, []string{"status"}), // success|failure
		recombineLatency: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Name:    "recombiner_recombine_duration_seconds",
			Unit:    "seconds",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
			Help:    "seconds it takes to recombine one video",
		}),
	}
}
