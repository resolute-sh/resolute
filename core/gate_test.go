package core

import (
	"context"
	"sync"
	"testing"
	"time"
)

type gateInput struct {
	Value string
}

type gateOutput struct {
	Result string
}

func TestGate_BasicApproval(t *testing.T) {
	t.Parallel()

	// given
	fetchNode := NewNode("fetch", func(ctx context.Context, in gateInput) (gateOutput, error) {
		return gateOutput{Result: "fetched"}, nil
	}, gateInput{Value: "data"})

	processNode := NewNode("process", func(ctx context.Context, in gateInput) (gateOutput, error) {
		return gateOutput{Result: "processed"}, nil
	}, gateInput{Value: "data"})

	flow := NewFlow("gate-flow").
		TriggeredBy(Manual("test")).
		Then(fetchNode).
		ThenGate("review", GateConfig{SignalName: "review_decision"}).
		Then(processNode).
		Build()

	tester := NewFlowTester().
		MockValue("fetch", gateOutput{Result: "fetched"}).
		MockGate("review", GateResult{
			Approved:  true,
			Decision:  "approved",
			DecidedBy: "user@example.com",
			Reason:    "looks good",
		}).
		MockValue("process", gateOutput{Result: "processed"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "fetch")
	tester.AssertCalled(t, "review")
	tester.AssertCalled(t, "process")

	result := Get[GateResult](state, "review")
	if !result.Approved {
		t.Error("expected gate to be approved")
	}
	if result.Decision != "approved" {
		t.Errorf("Decision: got %q, want %q", result.Decision, "approved")
	}
	if result.DecidedBy != "user@example.com" {
		t.Errorf("DecidedBy: got %q, want %q", result.DecidedBy, "user@example.com")
	}
	if result.DecidedAt.IsZero() {
		t.Error("DecidedAt should be set")
	}
}

func TestGate_Rejection(t *testing.T) {
	t.Parallel()

	// given
	fetchNode := NewNode("fetch", func(ctx context.Context, in gateInput) (gateOutput, error) {
		return gateOutput{}, nil
	}, gateInput{})

	processNode := NewNode("process", func(ctx context.Context, in gateInput) (gateOutput, error) {
		return gateOutput{}, nil
	}, gateInput{})

	flow := NewFlow("rejection-flow").
		TriggeredBy(Manual("test")).
		Then(fetchNode).
		ThenGate("approval", GateConfig{SignalName: "approval_signal"}).
		Then(processNode).
		Build()

	tester := NewFlowTester().
		MockValue("fetch", gateOutput{}).
		MockGate("approval", GateResult{
			Approved: false,
			Decision: "rejected",
			Reason:   "needs changes",
		}).
		MockValue("process", gateOutput{})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := Get[GateResult](state, "approval")
	if result.Approved {
		t.Error("expected gate to be rejected")
	}
	if result.Decision != "rejected" {
		t.Errorf("Decision: got %q, want %q", result.Decision, "rejected")
	}
}

func TestGate_UnmockedGateFails(t *testing.T) {
	t.Parallel()

	// given
	flow := NewFlow("unmocked-gate").
		TriggeredBy(Manual("test")).
		ThenGate("missing", GateConfig{SignalName: "signal"}).
		Build()

	tester := NewFlowTester()

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err == nil {
		t.Fatal("expected error for unmocked gate, got nil")
	}

	expected := `no gate mock registered for "missing"`
	if err.Error() != expected {
		t.Errorf("error: got %q, want %q", err.Error(), expected)
	}
}

func TestGate_MultipleGates(t *testing.T) {
	t.Parallel()

	// given
	node1 := NewNode("step1", func(ctx context.Context, in gateInput) (gateOutput, error) {
		return gateOutput{Result: "1"}, nil
	}, gateInput{})

	node2 := NewNode("step2", func(ctx context.Context, in gateInput) (gateOutput, error) {
		return gateOutput{Result: "2"}, nil
	}, gateInput{})

	node3 := NewNode("step3", func(ctx context.Context, in gateInput) (gateOutput, error) {
		return gateOutput{Result: "3"}, nil
	}, gateInput{})

	flow := NewFlow("multi-gate").
		TriggeredBy(Manual("test")).
		Then(node1).
		ThenGate("gate-1", GateConfig{SignalName: "signal_1"}).
		Then(node2).
		ThenGate("gate-2", GateConfig{SignalName: "signal_2"}).
		Then(node3).
		Build()

	tester := NewFlowTester().
		MockValue("step1", gateOutput{Result: "1"}).
		MockGate("gate-1", GateResult{Approved: true, Decision: "go"}).
		MockValue("step2", gateOutput{Result: "2"}).
		MockGate("gate-2", GateResult{Approved: true, Decision: "ship"}).
		MockValue("step3", gateOutput{Result: "3"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "step1")
	tester.AssertCalled(t, "gate-1")
	tester.AssertCalled(t, "step2")
	tester.AssertCalled(t, "gate-2")
	tester.AssertCalled(t, "step3")

	g1 := Get[GateResult](state, "gate-1")
	if g1.Decision != "go" {
		t.Errorf("gate-1 decision: got %q, want %q", g1.Decision, "go")
	}

	g2 := Get[GateResult](state, "gate-2")
	if g2.Decision != "ship" {
		t.Errorf("gate-2 decision: got %q, want %q", g2.Decision, "ship")
	}
}

func TestGate_WithHooks(t *testing.T) {
	t.Parallel()

	// given
	var mu sync.Mutex
	var trace []string
	record := func(event string) {
		mu.Lock()
		trace = append(trace, event)
		mu.Unlock()
	}

	hooks := &FlowHooks{
		BeforeStep: func(hc HookContext) { record("BeforeStep:" + hc.StepName) },
		AfterStep:  func(hc HookContext) { record("AfterStep:" + hc.StepName) },
		BeforeNode: func(hc HookContext) { record("BeforeNode:" + hc.NodeName) },
		AfterNode:  func(hc HookContext) { record("AfterNode:" + hc.NodeName) },
	}

	flow := NewFlow("gate-hooks").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		ThenGate("review", GateConfig{SignalName: "review_signal"}).
		Build()

	tester := NewFlowTester().
		MockGate("review", GateResult{Approved: true})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	expected := []string{
		"BeforeStep:review",
		"BeforeNode:review",
		"AfterNode:review",
		"AfterStep:review",
	}

	if len(trace) != len(expected) {
		t.Fatalf("trace length: got %d, want %d\ntrace: %v", len(trace), len(expected), trace)
	}

	for i, want := range expected {
		if trace[i] != want {
			t.Errorf("trace[%d]: got %q, want %q", i, trace[i], want)
		}
	}
}

func TestGate_GateResultMetadata(t *testing.T) {
	t.Parallel()

	// given
	flow := NewFlow("meta-gate").
		TriggeredBy(Manual("test")).
		ThenGate("review", GateConfig{SignalName: "sig"}).
		Build()

	tester := NewFlowTester().
		MockGate("review", GateResult{
			Approved: true,
			Metadata: map[string]string{
				"reviewer": "alice",
				"ticket":   "PROJ-123",
			},
		})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := Get[GateResult](state, "review")
	if result.Metadata["reviewer"] != "alice" {
		t.Errorf("Metadata[reviewer]: got %q, want %q", result.Metadata["reviewer"], "alice")
	}
	if result.Metadata["ticket"] != "PROJ-123" {
		t.Errorf("Metadata[ticket]: got %q, want %q", result.Metadata["ticket"], "PROJ-123")
	}
}

func TestGate_NodeCreation(t *testing.T) {
	t.Parallel()

	// given
	gate := NewGateNode("approval", GateConfig{
		SignalName: "approval_signal",
		Timeout:    24 * time.Hour,
	})

	// then
	if gate.Name() != "approval" {
		t.Errorf("Name: got %q, want %q", gate.Name(), "approval")
	}
	if gate.OutputKey() != "approval" {
		t.Errorf("OutputKey: got %q, want %q", gate.OutputKey(), "approval")
	}
	if gate.HasCompensation() {
		t.Error("expected HasCompensation() to return false")
	}
	if gate.Compensation() != nil {
		t.Error("expected Compensation() to return nil")
	}
	if gate.Input() != nil {
		t.Error("expected Input() to return nil")
	}
	if gate.RateLimiterID() != "" {
		t.Error("expected RateLimiterID() to return empty")
	}
}

func TestGate_CustomOutputKey(t *testing.T) {
	t.Parallel()

	// given
	gate := NewGateNode("approval", GateConfig{SignalName: "sig"}).As("custom-key")

	if gate.OutputKey() != "custom-key" {
		t.Errorf("OutputKey: got %q, want %q", gate.OutputKey(), "custom-key")
	}
}

func TestGate_TimeoutError(t *testing.T) {
	t.Parallel()

	// given
	err := &GateTimeoutError{GateName: "review", Timeout: 24 * time.Hour}

	// then
	expected := `gate "review" timed out after 24h0m0s`
	if err.Error() != expected {
		t.Errorf("Error: got %q, want %q", err.Error(), expected)
	}
}

func TestGate_BuilderValidation(t *testing.T) {
	t.Parallel()

	// given — empty signal name should produce build error
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty SignalName, got none")
		}
	}()

	NewFlow("bad-gate").
		TriggeredBy(Manual("test")).
		ThenGate("review", GateConfig{SignalName: ""}).
		Build()
}
