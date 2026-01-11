package core

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// PrometheusExporter implements MetricsExporter for Prometheus.
// It lazily creates and caches metric collectors on first use.
type PrometheusExporter struct {
	mu         sync.RWMutex
	counters   map[string]*prometheus.CounterVec
	histograms map[string]*prometheus.HistogramVec
}

// NewPrometheusExporter creates a new Prometheus metrics exporter.
// The exporter registers metrics with the default Prometheus registry.
func NewPrometheusExporter() *PrometheusExporter {
	e := &PrometheusExporter{
		counters:   make(map[string]*prometheus.CounterVec),
		histograms: make(map[string]*prometheus.HistogramVec),
	}

	e.counters["resolute_flow_executions_total"] = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "resolute_flow_executions_total",
			Help: "Total number of flow executions",
		},
		[]string{"flow", "status"},
	)

	e.counters["resolute_activity_errors_total"] = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "resolute_activity_errors_total",
			Help: "Total number of activity errors by type",
		},
		[]string{"node", "error_type"},
	)

	e.counters["resolute_state_operations_total"] = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "resolute_state_operations_total",
			Help: "Total number of state backend operations",
		},
		[]string{"operation"},
	)

	e.histograms["resolute_flow_duration_seconds"] = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "resolute_flow_duration_seconds",
			Help:    "Flow execution duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		},
		[]string{"flow"},
	)

	e.histograms["resolute_activity_duration_seconds"] = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "resolute_activity_duration_seconds",
			Help:    "Activity execution duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 12),
		},
		[]string{"node"},
	)

	e.histograms["resolute_rate_limiter_wait_seconds"] = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "resolute_rate_limiter_wait_seconds",
			Help:    "Time spent waiting for rate limiter in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15),
		},
		[]string{"limiter"},
	)

	return e
}

// CounterInc increments a counter metric.
func (e *PrometheusExporter) CounterInc(name string, labels map[string]string) {
	e.mu.RLock()
	counter, ok := e.counters[name]
	e.mu.RUnlock()

	if ok {
		counter.With(labels).Inc()
	}
}

// HistogramObserve records a value in a histogram metric.
func (e *PrometheusExporter) HistogramObserve(name string, value float64, labels map[string]string) {
	e.mu.RLock()
	histogram, ok := e.histograms[name]
	e.mu.RUnlock()

	if ok {
		histogram.With(labels).Observe(value)
	}
}
