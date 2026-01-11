package core

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type mockExporter struct {
	mu         sync.Mutex
	counters   map[string]int
	histograms map[string][]float64
	labels     map[string]map[string]string
}

func newMockExporter() *mockExporter {
	return &mockExporter{
		counters:   make(map[string]int),
		histograms: make(map[string][]float64),
		labels:     make(map[string]map[string]string),
	}
}

func (m *mockExporter) CounterInc(name string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name]++
	m.labels[name] = labels
}

func (m *mockExporter) HistogramObserve(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.histograms[name] = append(m.histograms[name], value)
	m.labels[name] = labels
}

func (m *mockExporter) getCounter(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counters[name]
}

func (m *mockExporter) getHistogramCount(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.histograms[name])
}

func (m *mockExporter) getLabels(name string) map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.labels[name]
}

func TestSetMetricsExporter(t *testing.T) {
	t.Cleanup(func() {
		ClearMetricsExporter()
	})

	exporter := newMockExporter()
	SetMetricsExporter(exporter)

	got := GetMetricsExporter()
	if got != exporter {
		t.Error("GetMetricsExporter did not return the set exporter")
	}
}

func TestClearMetricsExporter(t *testing.T) {
	exporter := newMockExporter()
	SetMetricsExporter(exporter)
	ClearMetricsExporter()

	got := GetMetricsExporter()
	if got != nil {
		t.Error("ClearMetricsExporter did not clear the exporter")
	}
}

func TestRecordFlowExecution(t *testing.T) {
	t.Cleanup(func() {
		ClearMetricsExporter()
	})

	tests := []struct {
		name     string
		input    RecordFlowExecutionInput
		wantFlow string
		wantStatus string
	}{
		{
			name: "success",
			input: RecordFlowExecutionInput{
				FlowName: "test-flow",
				Status:   "success",
				Duration: 100 * time.Millisecond,
			},
			wantFlow:   "test-flow",
			wantStatus: "success",
		},
		{
			name: "error",
			input: RecordFlowExecutionInput{
				FlowName: "error-flow",
				Status:   "error",
				Duration: 50 * time.Millisecond,
			},
			wantFlow:   "error-flow",
			wantStatus: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := newMockExporter()
			SetMetricsExporter(exporter)

			RecordFlowExecution(tt.input)

			if got := exporter.getCounter("resolute_flow_executions_total"); got != 1 {
				t.Errorf("counter = %d, want 1", got)
			}

			if got := exporter.getHistogramCount("resolute_flow_duration_seconds"); got != 1 {
				t.Errorf("histogram count = %d, want 1", got)
			}

			labels := exporter.getLabels("resolute_flow_executions_total")
			if labels["flow"] != tt.wantFlow {
				t.Errorf("flow label = %s, want %s", labels["flow"], tt.wantFlow)
			}
			if labels["status"] != tt.wantStatus {
				t.Errorf("status label = %s, want %s", labels["status"], tt.wantStatus)
			}
		})
	}
}

func TestRecordFlowExecution_NoExporter(t *testing.T) {
	ClearMetricsExporter()

	RecordFlowExecution(RecordFlowExecutionInput{
		FlowName: "test-flow",
		Status:   "success",
		Duration: 100 * time.Millisecond,
	})
}

func TestRecordActivityExecution(t *testing.T) {
	t.Cleanup(func() {
		ClearMetricsExporter()
	})

	tests := []struct {
		name       string
		input      RecordActivityExecutionInput
		wantNode   string
		wantError  bool
	}{
		{
			name: "success",
			input: RecordActivityExecutionInput{
				NodeName: "test-node",
				Duration: 50 * time.Millisecond,
				Err:      nil,
			},
			wantNode:  "test-node",
			wantError: false,
		},
		{
			name: "with_error",
			input: RecordActivityExecutionInput{
				NodeName: "error-node",
				Duration: 25 * time.Millisecond,
				Err:      errors.New("test error"),
			},
			wantNode:  "error-node",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := newMockExporter()
			SetMetricsExporter(exporter)

			RecordActivityExecution(tt.input)

			if got := exporter.getHistogramCount("resolute_activity_duration_seconds"); got != 1 {
				t.Errorf("histogram count = %d, want 1", got)
			}

			labels := exporter.getLabels("resolute_activity_duration_seconds")
			if labels["node"] != tt.wantNode {
				t.Errorf("node label = %s, want %s", labels["node"], tt.wantNode)
			}

			errorCount := exporter.getCounter("resolute_activity_errors_total")
			if tt.wantError && errorCount != 1 {
				t.Errorf("error counter = %d, want 1", errorCount)
			}
			if !tt.wantError && errorCount != 0 {
				t.Errorf("error counter = %d, want 0", errorCount)
			}
		})
	}
}

func TestRecordActivityExecution_ClassifiedError(t *testing.T) {
	t.Cleanup(func() {
		ClearMetricsExporter()
	})

	exporter := newMockExporter()
	SetMetricsExporter(exporter)

	classifiedErr := &ClassifiedError{
		Err:  errors.New("terminal error"),
		Type: ErrorTypeTerminal,
	}

	RecordActivityExecution(RecordActivityExecutionInput{
		NodeName: "test-node",
		Duration: 50 * time.Millisecond,
		Err:      classifiedErr,
	})

	labels := exporter.getLabels("resolute_activity_errors_total")
	if labels["error_type"] != "terminal" {
		t.Errorf("error_type label = %s, want terminal", labels["error_type"])
	}
}

func TestRecordRateLimitWait(t *testing.T) {
	t.Cleanup(func() {
		ClearMetricsExporter()
	})

	exporter := newMockExporter()
	SetMetricsExporter(exporter)

	RecordRateLimitWait(RecordRateLimitWaitInput{
		LimiterID: "test-limiter",
		WaitTime:  10 * time.Millisecond,
	})

	if got := exporter.getHistogramCount("resolute_rate_limiter_wait_seconds"); got != 1 {
		t.Errorf("histogram count = %d, want 1", got)
	}

	labels := exporter.getLabels("resolute_rate_limiter_wait_seconds")
	if labels["limiter"] != "test-limiter" {
		t.Errorf("limiter label = %s, want test-limiter", labels["limiter"])
	}
}

func TestRecordStateOperation(t *testing.T) {
	t.Cleanup(func() {
		ClearMetricsExporter()
	})

	exporter := newMockExporter()
	SetMetricsExporter(exporter)

	RecordStateOperation(RecordStateOperationInput{
		Operation: "load",
	})

	if got := exporter.getCounter("resolute_state_operations_total"); got != 1 {
		t.Errorf("counter = %d, want 1", got)
	}

	labels := exporter.getLabels("resolute_state_operations_total")
	if labels["operation"] != "load" {
		t.Errorf("operation label = %s, want load", labels["operation"])
	}
}

func TestNoopExporter(t *testing.T) {
	exporter := &NoopExporter{}

	exporter.CounterInc("test", map[string]string{"key": "value"})
	exporter.HistogramObserve("test", 1.0, map[string]string{"key": "value"})
}
