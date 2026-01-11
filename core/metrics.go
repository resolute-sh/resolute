package core

import (
	"errors"
	"sync"
	"time"
)

// MetricsExporter defines the interface for exporting metrics to backends.
// Implementations can target Prometheus, OpenTelemetry, or other systems.
type MetricsExporter interface {
	CounterInc(name string, labels map[string]string)
	HistogramObserve(name string, value float64, labels map[string]string)
}

// Metrics provides the global metrics recording interface.
type Metrics struct {
	mu       sync.RWMutex
	exporter MetricsExporter
}

var globalMetrics = &Metrics{}

// SetMetricsExporter configures the global metrics exporter.
// Call this during application initialization before starting workflows.
func SetMetricsExporter(exporter MetricsExporter) {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()
	globalMetrics.exporter = exporter
}

// GetMetricsExporter returns the currently configured metrics exporter.
func GetMetricsExporter() MetricsExporter {
	globalMetrics.mu.RLock()
	defer globalMetrics.mu.RUnlock()
	return globalMetrics.exporter
}

// ClearMetricsExporter removes the global metrics exporter (for testing).
func ClearMetricsExporter() {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()
	globalMetrics.exporter = nil
}

// RecordFlowExecutionInput holds parameters for recording flow execution metrics.
type RecordFlowExecutionInput struct {
	FlowName string
	Status   string
	Duration time.Duration
}

// RecordFlowExecution records metrics for a completed flow execution.
func RecordFlowExecution(input RecordFlowExecutionInput) {
	globalMetrics.mu.RLock()
	exporter := globalMetrics.exporter
	globalMetrics.mu.RUnlock()

	if exporter == nil {
		return
	}

	exporter.CounterInc("resolute_flow_executions_total", map[string]string{
		"flow":   input.FlowName,
		"status": input.Status,
	})

	exporter.HistogramObserve("resolute_flow_duration_seconds", input.Duration.Seconds(), map[string]string{
		"flow": input.FlowName,
	})
}

// RecordActivityExecutionInput holds parameters for recording activity execution metrics.
type RecordActivityExecutionInput struct {
	NodeName string
	Duration time.Duration
	Err      error
}

// RecordActivityExecution records metrics for a completed activity execution.
func RecordActivityExecution(input RecordActivityExecutionInput) {
	globalMetrics.mu.RLock()
	exporter := globalMetrics.exporter
	globalMetrics.mu.RUnlock()

	if exporter == nil {
		return
	}

	exporter.HistogramObserve("resolute_activity_duration_seconds", input.Duration.Seconds(), map[string]string{
		"node": input.NodeName,
	})

	if input.Err != nil {
		errType := "unknown"
		var classifiedErr *ClassifiedError
		if errors.As(input.Err, &classifiedErr) {
			errType = classifiedErr.Type.String()
		}
		exporter.CounterInc("resolute_activity_errors_total", map[string]string{
			"node":       input.NodeName,
			"error_type": errType,
		})
	}
}

// RecordRateLimitWaitInput holds parameters for recording rate limit wait metrics.
type RecordRateLimitWaitInput struct {
	LimiterID string
	WaitTime  time.Duration
}

// RecordRateLimitWait records metrics for rate limiter wait time.
func RecordRateLimitWait(input RecordRateLimitWaitInput) {
	globalMetrics.mu.RLock()
	exporter := globalMetrics.exporter
	globalMetrics.mu.RUnlock()

	if exporter == nil {
		return
	}

	exporter.HistogramObserve("resolute_rate_limiter_wait_seconds", input.WaitTime.Seconds(), map[string]string{
		"limiter": input.LimiterID,
	})
}

// RecordStateOperationInput holds parameters for recording state operation metrics.
type RecordStateOperationInput struct {
	Operation string
}

// RecordStateOperation records metrics for state backend operations.
func RecordStateOperation(input RecordStateOperationInput) {
	globalMetrics.mu.RLock()
	exporter := globalMetrics.exporter
	globalMetrics.mu.RUnlock()

	if exporter == nil {
		return
	}

	exporter.CounterInc("resolute_state_operations_total", map[string]string{
		"operation": input.Operation,
	})
}

// NoopExporter is a metrics exporter that does nothing.
// Useful for testing or when metrics are disabled.
type NoopExporter struct{}

func (n *NoopExporter) CounterInc(name string, labels map[string]string)               {}
func (n *NoopExporter) HistogramObserve(name string, value float64, labels map[string]string) {}
