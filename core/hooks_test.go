package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type hookInput struct {
	Value string
}

type hookOutput struct {
	Result string
}

func TestHooks_InvocationOrder(t *testing.T) {
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
		BeforeFlow: func(hc HookContext) { record("BeforeFlow:" + hc.FlowName) },
		AfterFlow:  func(hc HookContext) { record("AfterFlow:" + hc.FlowName) },
		BeforeStep: func(hc HookContext) { record("BeforeStep:" + hc.StepName) },
		AfterStep:  func(hc HookContext) { record("AfterStep:" + hc.StepName) },
		BeforeNode: func(hc HookContext) { record("BeforeNode:" + hc.NodeName) },
		AfterNode:  func(hc HookContext) { record("AfterNode:" + hc.NodeName) },
	}

	node1 := NewNode("fetch", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{Result: "fetched"}, nil
	}, hookInput{Value: "a"})

	node2 := NewNode("process", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{Result: "processed"}, nil
	}, hookInput{Value: "b"})

	flow := NewFlow("hook-flow").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		Then(node1).
		Then(node2).
		Build()

	tester := NewFlowTester().
		MockValue("fetch", hookOutput{Result: "fetched"}).
		MockValue("process", hookOutput{Result: "processed"})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{
		"BeforeFlow:hook-flow",
		"BeforeStep:fetch",
		"BeforeNode:fetch",
		"AfterNode:fetch",
		"AfterStep:fetch",
		"BeforeStep:process",
		"BeforeNode:process",
		"AfterNode:process",
		"AfterStep:process",
		"AfterFlow:hook-flow",
	}

	mu.Lock()
	defer mu.Unlock()

	if len(trace) != len(expected) {
		t.Fatalf("trace length: got %d, want %d\ntrace: %v", len(trace), len(expected), trace)
	}

	for i, want := range expected {
		if trace[i] != want {
			t.Errorf("trace[%d]: got %q, want %q", i, trace[i], want)
		}
	}
}

func TestHooks_NilSafety(t *testing.T) {
	t.Parallel()

	// given — flow with no hooks (nil)
	node := NewNode("step", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{Result: "ok"}, nil
	}, hookInput{Value: "x"})

	flow := NewFlow("no-hooks").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester().
		MockValue("step", hookOutput{Result: "ok"})

	// when — should not panic
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := Get[hookOutput](state, "step")
	if result.Result != "ok" {
		t.Errorf("result: got %q, want %q", result.Result, "ok")
	}
}

func TestHooks_PartialNilSafety(t *testing.T) {
	t.Parallel()

	// given — hooks struct with only some callbacks set
	var called bool
	hooks := &FlowHooks{
		AfterFlow: func(hc HookContext) { called = true },
	}

	node := NewNode("step", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{Result: "ok"}, nil
	}, hookInput{Value: "x"})

	flow := NewFlow("partial-hooks").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		Then(node).
		Build()

	tester := NewFlowTester().
		MockValue("step", hookOutput{Result: "ok"})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("AfterFlow hook was not called")
	}
}

func TestHooks_AfterNodeReceivesDuration(t *testing.T) {
	t.Parallel()

	// given
	var capturedDuration time.Duration
	hooks := &FlowHooks{
		AfterNode: func(hc HookContext) {
			capturedDuration = hc.Duration
		},
	}

	node := NewNode("slow-step", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{}, nil
	}, hookInput{})

	flow := NewFlow("duration-flow").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		Then(node).
		Build()

	tester := NewFlowTester().
		MockValue("slow-step", hookOutput{})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedDuration < 0 {
		t.Errorf("duration should be non-negative, got %v", capturedDuration)
	}
}

func TestHooks_AfterNodeReceivesError(t *testing.T) {
	t.Parallel()

	// given
	var capturedErr error
	hooks := &FlowHooks{
		AfterNode: func(hc HookContext) {
			capturedErr = hc.Error
		},
	}

	expectedErr := errors.New("node failed")

	node := NewNode("failing-step", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{}, nil
	}, hookInput{})

	flow := NewFlow("error-flow").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		Then(node).
		Build()

	tester := NewFlowTester().
		MockError("failing-step", expectedErr)

	// when
	_, _ = tester.Run(flow, FlowInput{})

	// then
	if capturedErr == nil {
		t.Fatal("expected AfterNode to receive error, got nil")
	}
	if !errors.Is(capturedErr, expectedErr) {
		t.Errorf("AfterNode error: got %v, want %v", capturedErr, expectedErr)
	}
}

func TestHooks_AfterFlowReceivesError(t *testing.T) {
	t.Parallel()

	// given
	var capturedErr error
	hooks := &FlowHooks{
		AfterFlow: func(hc HookContext) {
			capturedErr = hc.Error
		},
	}

	expectedErr := errors.New("flow failed")

	node := NewNode("bad-step", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{}, nil
	}, hookInput{})

	flow := NewFlow("fail-flow").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		Then(node).
		Build()

	tester := NewFlowTester().
		MockError("bad-step", expectedErr)

	// when
	_, _ = tester.Run(flow, FlowInput{})

	// then
	if capturedErr == nil {
		t.Fatal("expected AfterFlow to receive error, got nil")
	}
	if !errors.Is(capturedErr, expectedErr) {
		t.Errorf("AfterFlow error: got %v, want %v", capturedErr, expectedErr)
	}
}

func TestHooks_OnCost(t *testing.T) {
	t.Parallel()

	// given
	var captured CostEntry
	hooks := &FlowHooks{
		OnCost: func(entry CostEntry) {
			captured = entry
		},
	}

	entry := CostEntry{
		NodeName:  "llm-call",
		Model:     "claude-sonnet-4-20250514",
		Provider:  "anthropic",
		TokensIn:  1000,
		TokensOut: 500,
		CostUSD:   0.0045,
		Duration:  2 * time.Second,
		Metadata:  map[string]string{"session": "abc123"},
	}

	// when
	invokeCost(hooks, entry)

	// then
	if captured.NodeName != "llm-call" {
		t.Errorf("NodeName: got %q, want %q", captured.NodeName, "llm-call")
	}
	if captured.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model: got %q, want %q", captured.Model, "claude-sonnet-4-20250514")
	}
	if captured.TokensIn != 1000 {
		t.Errorf("TokensIn: got %d, want %d", captured.TokensIn, 1000)
	}
	if captured.TokensOut != 500 {
		t.Errorf("TokensOut: got %d, want %d", captured.TokensOut, 500)
	}
	if captured.CostUSD != 0.0045 {
		t.Errorf("CostUSD: got %f, want %f", captured.CostUSD, 0.0045)
	}
	if captured.Metadata["session"] != "abc123" {
		t.Errorf("Metadata[session]: got %q, want %q", captured.Metadata["session"], "abc123")
	}
}

func TestHooks_OnCostNilSafety(t *testing.T) {
	t.Parallel()

	// should not panic
	invokeCost(nil, CostEntry{NodeName: "test"})

	hooks := &FlowHooks{}
	invokeCost(hooks, CostEntry{NodeName: "test"})
}

func TestHooks_ParallelStepHookOrder(t *testing.T) {
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

	p1 := NewNode("p1", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{}, nil
	}, hookInput{}).As("p1-out")

	p2 := NewNode("p2", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{}, nil
	}, hookInput{}).As("p2-out")

	flow := NewFlow("parallel-hooks").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		ThenParallel("batch", p1, p2).
		Build()

	tester := NewFlowTester().
		MockValue("p1", hookOutput{}).
		MockValue("p2", hookOutput{})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(trace) != 6 {
		t.Fatalf("trace length: got %d, want 6\ntrace: %v", len(trace), trace)
	}

	if trace[0] != "BeforeStep:batch" {
		t.Errorf("trace[0]: got %q, want %q", trace[0], "BeforeStep:batch")
	}
	if trace[len(trace)-1] != "AfterStep:batch" {
		t.Errorf("trace[last]: got %q, want %q", trace[len(trace)-1], "AfterStep:batch")
	}
}

func TestHooks_ConditionalBranch(t *testing.T) {
	t.Parallel()

	// given
	var mu sync.Mutex
	var nodeNames []string
	hooks := &FlowHooks{
		BeforeNode: func(hc HookContext) {
			mu.Lock()
			nodeNames = append(nodeNames, hc.NodeName)
			mu.Unlock()
		},
	}

	thenNode := NewNode("then-action", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{}, nil
	}, hookInput{})

	elseNode := NewNode("else-action", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{}, nil
	}, hookInput{})

	setupNode := NewNode("setup", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{}, nil
	}, hookInput{}).As("setup-result")

	flow := NewFlow("cond-hooks").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		Then(setupNode).
		When(func(s *FlowState) bool {
			return true
		}).
		Then(thenNode).
		Otherwise(elseNode).
		Build()

	tester := NewFlowTester().
		MockValue("setup", hookOutput{}).
		MockValue("then-action", hookOutput{}).
		MockValue("else-action", hookOutput{})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(nodeNames) != 2 {
		t.Fatalf("expected 2 BeforeNode calls, got %d: %v", len(nodeNames), nodeNames)
	}
	if nodeNames[0] != "setup" {
		t.Errorf("nodeNames[0]: got %q, want %q", nodeNames[0], "setup")
	}
	if nodeNames[1] != "then-action" {
		t.Errorf("nodeNames[1]: got %q, want %q", nodeNames[1], "then-action")
	}
}

func TestHooks_HookContextFields(t *testing.T) {
	t.Parallel()

	// given
	var captured HookContext
	hooks := &FlowHooks{
		BeforeNode: func(hc HookContext) {
			captured = hc
		},
	}

	node := NewNode("my-node", func(ctx context.Context, in hookInput) (hookOutput, error) {
		return hookOutput{}, nil
	}, hookInput{})

	flow := NewFlow("context-flow").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		Then(node).
		Build()

	tester := NewFlowTester().
		MockValue("my-node", hookOutput{})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured.FlowName != "context-flow" {
		t.Errorf("FlowName: got %q, want %q", captured.FlowName, "context-flow")
	}
	if captured.StepName != "my-node" {
		t.Errorf("StepName: got %q, want %q", captured.StepName, "my-node")
	}
	if captured.NodeName != "my-node" {
		t.Errorf("NodeName: got %q, want %q", captured.NodeName, "my-node")
	}
}
