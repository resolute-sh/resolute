package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

type childInput struct {
	Value string
}

type childOutput struct {
	Result string
}

func TestChildFlow_BasicExecution(t *testing.T) {
	t.Parallel()

	// given
	childFlow := NewFlow("child-flow").
		TriggeredBy(Manual("test")).
		Then(NewNode("child-step", func(ctx context.Context, in childInput) (childOutput, error) {
			return childOutput{}, nil
		}, childInput{})).
		Build()

	parentNode := NewNode("parent-step", func(ctx context.Context, in childInput) (childOutput, error) {
		return childOutput{}, nil
	}, childInput{})

	flow := NewFlow("parent-flow").
		TriggeredBy(Manual("test")).
		Then(parentNode).
		ThenChildren("fan-out", ChildFlowConfig{
			Flow: childFlow,
			InputMapper: func(s *FlowState) []FlowInput {
				return []FlowInput{{}, {}, {}}
			},
		}).
		Build()

	tester := NewFlowTester().
		MockValue("parent-step", childOutput{Result: "parent"}).
		MockChildFlow("fan-out", func(s *FlowState) (*FlowState, error) {
			result := NewFlowState(FlowInput{})
			Set(result, "fan-out-result", childOutput{Result: "children-done"})
			return result, nil
		})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "parent-step")
	tester.AssertCalled(t, "fan-out")

	result := Get[childOutput](state, "fan-out-result")
	if result.Result != "children-done" {
		t.Errorf("result: got %q, want %q", result.Result, "children-done")
	}
}

func TestChildFlow_ErrorPropagation(t *testing.T) {
	t.Parallel()

	// given
	childFlow := NewFlow("child-flow").
		TriggeredBy(Manual("test")).
		Then(NewNode("child-step", func(ctx context.Context, in childInput) (childOutput, error) {
			return childOutput{}, nil
		}, childInput{})).
		Build()

	expectedErr := errors.New("child failed")

	flow := NewFlow("parent-flow").
		TriggeredBy(Manual("test")).
		ThenChildren("fan-out", ChildFlowConfig{
			Flow: childFlow,
			InputMapper: func(s *FlowState) []FlowInput {
				return []FlowInput{{}}
			},
		}).
		Build()

	tester := NewFlowTester().
		MockChildFlow("fan-out", func(s *FlowState) (*FlowState, error) {
			return nil, expectedErr
		})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap %v, got %v", expectedErr, err)
	}
}

func TestChildFlow_UnmockedFails(t *testing.T) {
	t.Parallel()

	// given
	childFlow := NewFlow("child-flow").
		TriggeredBy(Manual("test")).
		Then(NewNode("child-step", func(ctx context.Context, in childInput) (childOutput, error) {
			return childOutput{}, nil
		}, childInput{})).
		Build()

	flow := NewFlow("parent-flow").
		TriggeredBy(Manual("test")).
		ThenChildren("missing-mock", ChildFlowConfig{
			Flow: childFlow,
			InputMapper: func(s *FlowState) []FlowInput {
				return []FlowInput{{}}
			},
		}).
		Build()

	tester := NewFlowTester()

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err == nil {
		t.Fatal("expected error for unmocked child flow, got nil")
	}

	expected := `no child flow mock registered for "missing-mock"`
	if err.Error() != expected {
		t.Errorf("error: got %q, want %q", err.Error(), expected)
	}
}

func TestChildFlow_WithHooks(t *testing.T) {
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
	}

	childFlow := NewFlow("child-flow").
		TriggeredBy(Manual("test")).
		Then(NewNode("child-step", func(ctx context.Context, in childInput) (childOutput, error) {
			return childOutput{}, nil
		}, childInput{})).
		Build()

	flow := NewFlow("parent-flow").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		ThenChildren("spawn", ChildFlowConfig{
			Flow: childFlow,
			InputMapper: func(s *FlowState) []FlowInput {
				return []FlowInput{{}}
			},
		}).
		Build()

	tester := NewFlowTester().
		MockChildFlow("spawn", func(s *FlowState) (*FlowState, error) {
			return nil, nil
		})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	expected := []string{
		"BeforeStep:spawn",
		"AfterStep:spawn",
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

func TestChildFlow_NodeCreation(t *testing.T) {
	t.Parallel()

	// given
	childFlow := NewFlow("child").
		TriggeredBy(Manual("test")).
		Then(NewNode("step", func(ctx context.Context, in childInput) (childOutput, error) {
			return childOutput{}, nil
		}, childInput{})).
		Build()

	node := NewChildFlowNode("spawner", ChildFlowConfig{
		Flow: childFlow,
		InputMapper: func(s *FlowState) []FlowInput {
			return []FlowInput{{}}
		},
	})

	// then
	if node.Name() != "spawner" {
		t.Errorf("Name: got %q, want %q", node.Name(), "spawner")
	}
	if node.OutputKey() != "spawner" {
		t.Errorf("OutputKey: got %q, want %q", node.OutputKey(), "spawner")
	}
	if node.HasCompensation() {
		t.Error("expected HasCompensation() to return false")
	}
	if node.Compensation() != nil {
		t.Error("expected Compensation() to return nil")
	}
	if node.Input() != nil {
		t.Error("expected Input() to return nil")
	}
	if node.RateLimiterID() != "" {
		t.Error("expected RateLimiterID() to return empty")
	}
}

func TestChildFlow_CustomOutputKey(t *testing.T) {
	t.Parallel()

	// given
	childFlow := NewFlow("child").
		TriggeredBy(Manual("test")).
		Then(NewNode("step", func(ctx context.Context, in childInput) (childOutput, error) {
			return childOutput{}, nil
		}, childInput{})).
		Build()

	node := NewChildFlowNode("spawner", ChildFlowConfig{
		Flow: childFlow,
		InputMapper: func(s *FlowState) []FlowInput {
			return nil
		},
	}).As("custom-key")

	// then
	if node.OutputKey() != "custom-key" {
		t.Errorf("OutputKey: got %q, want %q", node.OutputKey(), "custom-key")
	}
}

func TestChildFlow_BuilderValidation_NilFlow(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil Flow, got none")
		}
	}()

	NewFlow("bad-children").
		TriggeredBy(Manual("test")).
		ThenChildren("spawn", ChildFlowConfig{
			Flow: nil,
			InputMapper: func(s *FlowState) []FlowInput {
				return nil
			},
		}).
		Build()
}

func TestChildFlow_BuilderValidation_NilMapper(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil InputMapper, got none")
		}
	}()

	childFlow := NewFlow("child").
		TriggeredBy(Manual("test")).
		Then(NewNode("step", func(ctx context.Context, in childInput) (childOutput, error) {
			return childOutput{}, nil
		}, childInput{})).
		Build()

	NewFlow("bad-children").
		TriggeredBy(Manual("test")).
		ThenChildren("spawn", ChildFlowConfig{
			Flow:        childFlow,
			InputMapper: nil,
		}).
		Build()
}

func TestChildFlow_WithGatesAndChildren(t *testing.T) {
	t.Parallel()

	// given — integration test: gate → children → final step
	childFlow := NewFlow("child-flow").
		TriggeredBy(Manual("test")).
		Then(NewNode("child-work", func(ctx context.Context, in childInput) (childOutput, error) {
			return childOutput{}, nil
		}, childInput{})).
		Build()

	setupNode := NewNode("setup", func(ctx context.Context, in childInput) (childOutput, error) {
		return childOutput{}, nil
	}, childInput{})

	finalNode := NewNode("aggregate", func(ctx context.Context, in childInput) (childOutput, error) {
		return childOutput{}, nil
	}, childInput{})

	flow := NewFlow("full-pipeline").
		TriggeredBy(Manual("test")).
		Then(setupNode).
		ThenGate("review", GateConfig{SignalName: "review_signal"}).
		ThenChildren("fan-out", ChildFlowConfig{
			Flow: childFlow,
			InputMapper: func(s *FlowState) []FlowInput {
				return []FlowInput{{}, {}}
			},
		}).
		Then(finalNode).
		Build()

	tester := NewFlowTester().
		MockValue("setup", childOutput{Result: "setup-done"}).
		MockGate("review", GateResult{Approved: true}).
		MockChildFlow("fan-out", func(s *FlowState) (*FlowState, error) {
			result := NewFlowState(FlowInput{})
			Set(result, "child-results", childOutput{Result: fmt.Sprintf("processed-%d-children", 2)})
			return result, nil
		}).
		MockValue("aggregate", childOutput{Result: "aggregated"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "setup")
	tester.AssertCalled(t, "review")
	tester.AssertCalled(t, "fan-out")
	tester.AssertCalled(t, "aggregate")

	gate := Get[GateResult](state, "review")
	if !gate.Approved {
		t.Error("gate should be approved")
	}

	childResult := Get[childOutput](state, "child-results")
	if childResult.Result != "processed-2-children" {
		t.Errorf("child result: got %q, want %q", childResult.Result, "processed-2-children")
	}

	final := Get[childOutput](state, "aggregate")
	if final.Result != "aggregated" {
		t.Errorf("final result: got %q, want %q", final.Result, "aggregated")
	}
}
