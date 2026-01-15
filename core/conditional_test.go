package core

import (
	"context"
	"errors"
	"testing"
)

type condInput struct {
	Value int
}

type condOutput struct {
	Result string
}

func TestConditional_ThenBranchExecuted(t *testing.T) {
	t.Parallel()

	// given
	thenNode := NewNode("then-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{Result: "then"}, nil
	}, condInput{Value: 1})

	elseNode := NewNode("else-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{Result: "else"}, nil
	}, condInput{Value: 2})

	setupNode := NewNode("setup", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{Value: 100}).As("setup-result")

	flow := NewFlow("conditional-flow").
		TriggeredBy(Manual("test")).
		Then(setupNode).
		When(func(s *FlowState) bool {
			return Get[condOutput](s, "setup-result").Result == ""
		}).
		Then(thenNode).
		Otherwise(elseNode).
		Build()

	tester := NewFlowTester().
		MockValue("setup", condOutput{Result: ""}).
		MockValue("then-action", condOutput{Result: "then-executed"}).
		MockValue("else-action", condOutput{Result: "else-executed"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "then-action")
	tester.AssertNotCalled(t, "else-action")

	result := Get[condOutput](state, "then-action")
	if result.Result != "then-executed" {
		t.Errorf("result: got %q, want %q", result.Result, "then-executed")
	}
}

func TestConditional_ElseBranchExecuted(t *testing.T) {
	t.Parallel()

	// given
	thenNode := NewNode("then-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{Result: "then"}, nil
	}, condInput{Value: 1})

	elseNode := NewNode("else-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{Result: "else"}, nil
	}, condInput{Value: 2})

	setupNode := NewNode("setup", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{Value: 100}).As("setup-result")

	flow := NewFlow("conditional-flow").
		TriggeredBy(Manual("test")).
		Then(setupNode).
		When(func(s *FlowState) bool {
			return Get[condOutput](s, "setup-result").Result == "match"
		}).
		Then(thenNode).
		Otherwise(elseNode).
		Build()

	tester := NewFlowTester().
		MockValue("setup", condOutput{Result: "no-match"}).
		MockValue("then-action", condOutput{Result: "then-executed"}).
		MockValue("else-action", condOutput{Result: "else-executed"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertNotCalled(t, "then-action")
	tester.AssertCalled(t, "else-action")

	result := Get[condOutput](state, "else-action")
	if result.Result != "else-executed" {
		t.Errorf("result: got %q, want %q", result.Result, "else-executed")
	}
}

func TestConditional_NoElseBranch(t *testing.T) {
	t.Parallel()

	// given
	thenNode := NewNode("then-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{Result: "then"}, nil
	}, condInput{})

	setupNode := NewNode("setup", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{}).As("setup-result")

	afterNode := NewNode("after", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	flow := NewFlow("no-else-flow").
		TriggeredBy(Manual("test")).
		Then(setupNode).
		When(func(s *FlowState) bool {
			return true
		}).
		Then(thenNode).
		EndWhen().
		Then(afterNode).
		Build()

	tester := NewFlowTester().
		MockValue("setup", condOutput{}).
		MockValue("then-action", condOutput{Result: "then-executed"}).
		MockValue("after", condOutput{Result: "after"})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "setup")
	tester.AssertCalled(t, "then-action")
	tester.AssertCalled(t, "after")
}

func TestConditional_NoElseBranchSkipped(t *testing.T) {
	t.Parallel()

	// given
	thenNode := NewNode("then-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	afterNode := NewNode("after", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	flow := NewFlow("skip-when-false").
		TriggeredBy(Manual("test")).
		When(func(s *FlowState) bool {
			return false
		}).
		Then(thenNode).
		EndWhen().
		Then(afterNode).
		Build()

	tester := NewFlowTester().
		MockValue("then-action", condOutput{}).
		MockValue("after", condOutput{})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertNotCalled(t, "then-action")
	tester.AssertCalled(t, "after")
}

func TestConditional_MultipleStepsInThenBranch(t *testing.T) {
	t.Parallel()

	// given
	node1 := NewNode("step1", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	node2 := NewNode("step2", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	node3 := NewNode("step3", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	flow := NewFlow("multi-step-then").
		TriggeredBy(Manual("test")).
		When(func(s *FlowState) bool {
			return true
		}).
		Then(node1).
		Then(node2).
		Then(node3).
		EndWhen().
		Build()

	tester := NewFlowTester().
		MockValue("step1", condOutput{Result: "1"}).
		MockValue("step2", condOutput{Result: "2"}).
		MockValue("step3", condOutput{Result: "3"})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "step1")
	tester.AssertCalled(t, "step2")
	tester.AssertCalled(t, "step3")
}

func TestConditional_MultipleStepsInElseBranch(t *testing.T) {
	t.Parallel()

	// given
	thenNode := NewNode("then-step", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	elseNode1 := NewNode("else1", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	elseNode2 := NewNode("else2", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	flow := NewFlow("multi-step-else").
		TriggeredBy(Manual("test")).
		When(func(s *FlowState) bool {
			return false
		}).
		Then(thenNode).
		Else().
		Then(elseNode1).
		Then(elseNode2).
		EndWhen().
		Build()

	tester := NewFlowTester().
		MockValue("then-step", condOutput{}).
		MockValue("else1", condOutput{Result: "e1"}).
		MockValue("else2", condOutput{Result: "e2"})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertNotCalled(t, "then-step")
	tester.AssertCalled(t, "else1")
	tester.AssertCalled(t, "else2")
}

func TestConditional_ParallelInBranch(t *testing.T) {
	t.Parallel()

	// given
	p1 := NewNode("parallel1", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{}).As("p1")

	p2 := NewNode("parallel2", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{}).As("p2")

	elseNode := NewNode("else-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	flow := NewFlow("parallel-in-conditional").
		TriggeredBy(Manual("test")).
		When(func(s *FlowState) bool {
			return true
		}).
		ThenParallel("parallel-step", p1, p2).
		Otherwise(elseNode).
		Build()

	tester := NewFlowTester().
		MockValue("parallel1", condOutput{Result: "p1"}).
		MockValue("parallel2", condOutput{Result: "p2"}).
		MockValue("else-action", condOutput{})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "parallel1")
	tester.AssertCalled(t, "parallel2")
	tester.AssertNotCalled(t, "else-action")

	r1 := Get[condOutput](state, "p1")
	if r1.Result != "p1" {
		t.Errorf("p1 result: got %q, want %q", r1.Result, "p1")
	}
}

func TestConditional_ErrorInThenBranch(t *testing.T) {
	t.Parallel()

	// given
	thenNode := NewNode("failing-then", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	elseNode := NewNode("else-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	flow := NewFlow("error-in-then").
		TriggeredBy(Manual("test")).
		When(func(s *FlowState) bool {
			return true
		}).
		Then(thenNode).
		Otherwise(elseNode).
		Build()

	expectedErr := errors.New("then branch error")

	tester := NewFlowTester().
		MockError("failing-then", expectedErr).
		MockValue("else-action", condOutput{})

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

func TestConditional_OtherwiseParallel(t *testing.T) {
	t.Parallel()

	// given
	thenNode := NewNode("then-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	ep1 := NewNode("else-parallel1", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{}).As("ep1")

	ep2 := NewNode("else-parallel2", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{}).As("ep2")

	flow := NewFlow("otherwise-parallel").
		TriggeredBy(Manual("test")).
		When(func(s *FlowState) bool {
			return false
		}).
		Then(thenNode).
		OtherwiseParallel("else-parallel", ep1, ep2).
		Build()

	tester := NewFlowTester().
		MockValue("then-action", condOutput{}).
		MockValue("else-parallel1", condOutput{Result: "ep1"}).
		MockValue("else-parallel2", condOutput{Result: "ep2"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertNotCalled(t, "then-action")
	tester.AssertCalled(t, "else-parallel1")
	tester.AssertCalled(t, "else-parallel2")

	r1 := Get[condOutput](state, "ep1")
	if r1.Result != "ep1" {
		t.Errorf("ep1 result: got %q, want %q", r1.Result, "ep1")
	}
}

func TestConditional_StepsAfterConditional(t *testing.T) {
	t.Parallel()

	// given
	thenNode := NewNode("then-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	elseNode := NewNode("else-action", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	afterNode := NewNode("after-conditional", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	flow := NewFlow("steps-after").
		TriggeredBy(Manual("test")).
		When(func(s *FlowState) bool {
			return true
		}).
		Then(thenNode).
		Otherwise(elseNode).
		Then(afterNode).
		Build()

	tester := NewFlowTester().
		MockValue("then-action", condOutput{Result: "then"}).
		MockValue("else-action", condOutput{}).
		MockValue("after-conditional", condOutput{Result: "after"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "then-action")
	tester.AssertNotCalled(t, "else-action")
	tester.AssertCalled(t, "after-conditional")

	result := Get[condOutput](state, "after-conditional")
	if result.Result != "after" {
		t.Errorf("after result: got %q, want %q", result.Result, "after")
	}
}

func TestConditional_PredicateAccessesState(t *testing.T) {
	t.Parallel()

	// given
	fetchNode := NewNode("fetch", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{}).As("fetched")

	processNode := NewNode("process", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	skipNode := NewNode("skip", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	flow := NewFlow("state-access").
		TriggeredBy(Manual("test")).
		Then(fetchNode).
		When(func(s *FlowState) bool {
			fetched := Get[condOutput](s, "fetched")
			return fetched.Result == "process-me"
		}).
		Then(processNode).
		Otherwise(skipNode).
		Build()

	tester := NewFlowTester().
		MockValue("fetch", condOutput{Result: "process-me"}).
		MockValue("process", condOutput{Result: "processed"}).
		MockValue("skip", condOutput{Result: "skipped"})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "fetch")
	tester.AssertCalled(t, "process")
	tester.AssertNotCalled(t, "skip")
}

func TestConditional_ContextCancellation(t *testing.T) {
	t.Parallel()

	// given
	node1 := NewNode("step1", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	node2 := NewNode("step2", func(ctx context.Context, in condInput) (condOutput, error) {
		return condOutput{}, nil
	}, condInput{})

	flow := NewFlow("cancellable").
		TriggeredBy(Manual("test")).
		When(func(s *FlowState) bool {
			return true
		}).
		Then(node1).
		Then(node2).
		EndWhen().
		Build()

	tester := NewFlowTester().
		MockValue("step1", condOutput{}).
		MockValue("step2", condOutput{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// when
	_, err := tester.RunWithContext(ctx, flow, FlowInput{})

	// then
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
