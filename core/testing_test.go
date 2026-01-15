package core

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// Test types for FlowTester tests
type testerInput struct {
	Value string
}

type testerOutput struct {
	Result string
	Count  int
}

func TestFlowTester_MockAndRun(t *testing.T) {
	t.Parallel()

	// Create a simple flow
	node1 := NewNode("step1", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{Result: in.Value, Count: 1}, nil
	}, testerInput{Value: "hello"})

	node2 := NewNode("step2", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{Result: in.Value, Count: 2}, nil
	}, testerInput{Value: "world"})

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node1).
		Then(node2).
		Build()

	// Create tester with mocks
	tester := NewFlowTester().
		Mock("step1", func(in testerInput) (testerOutput, error) {
			return testerOutput{Result: "mocked-" + in.Value, Count: 10}, nil
		}).
		Mock("step2", func(in testerInput) (testerOutput, error) {
			return testerOutput{Result: "mocked-" + in.Value, Count: 20}, nil
		})

	// Run the flow
	state, err := tester.Run(flow, FlowInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check results
	result1 := Get[testerOutput](state, "step1")
	if result1.Result != "mocked-hello" {
		t.Errorf("step1 result: got %q, want %q", result1.Result, "mocked-hello")
	}
	if result1.Count != 10 {
		t.Errorf("step1 count: got %d, want %d", result1.Count, 10)
	}

	result2 := Get[testerOutput](state, "step2")
	if result2.Result != "mocked-world" {
		t.Errorf("step2 result: got %q, want %q", result2.Result, "mocked-world")
	}
}

func TestFlowTester_MockValue(t *testing.T) {
	t.Parallel()

	node := NewNode("step1", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{Value: "input"})

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	expectedOutput := testerOutput{Result: "fixed", Count: 99}

	tester := NewFlowTester().
		MockValue("step1", expectedOutput)

	state, err := tester.Run(flow, FlowInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := Get[testerOutput](state, "step1")
	if result.Result != expectedOutput.Result {
		t.Errorf("result: got %q, want %q", result.Result, expectedOutput.Result)
	}
	if result.Count != expectedOutput.Count {
		t.Errorf("count: got %d, want %d", result.Count, expectedOutput.Count)
	}
}

func TestFlowTester_MockError(t *testing.T) {
	t.Parallel()

	node := NewNode("failing-step", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{Value: "input"})

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	expectedErr := errors.New("mock error")

	tester := NewFlowTester().
		MockError("failing-step", expectedErr)

	_, err := tester.Run(flow, FlowInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap %v, got %v", expectedErr, err)
	}
}

func TestFlowTester_UnmockedNodeFails(t *testing.T) {
	t.Parallel()

	node := NewNode("unmocked", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{})

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester() // No mocks registered

	_, err := tester.Run(flow, FlowInput{})
	if err == nil {
		t.Fatal("expected error for unmocked node, got nil")
	}

	expectedMsg := `no mock registered for node "unmocked"`
	if err.Error() != expectedMsg {
		t.Errorf("error message: got %q, want %q", err.Error(), expectedMsg)
	}
}

func TestFlowTester_ParallelSteps(t *testing.T) {
	t.Parallel()

	node1 := NewNode("parallel1", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{Result: "p1"}, nil
	}, testerInput{}).As("p1")

	node2 := NewNode("parallel2", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{Result: "p2"}, nil
	}, testerInput{}).As("p2")

	node3 := NewNode("parallel3", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{Result: "p3"}, nil
	}, testerInput{}).As("p3")

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		ThenParallel("parallel-step", node1, node2, node3).
		Build()

	tester := NewFlowTester().
		Mock("parallel1", func(in testerInput) (testerOutput, error) {
			return testerOutput{Result: "mock-p1"}, nil
		}).
		Mock("parallel2", func(in testerInput) (testerOutput, error) {
			return testerOutput{Result: "mock-p2"}, nil
		}).
		Mock("parallel3", func(in testerInput) (testerOutput, error) {
			return testerOutput{Result: "mock-p3"}, nil
		})

	state, err := tester.Run(flow, FlowInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All parallel nodes should have been called
	tester.AssertCalled(t, "parallel1")
	tester.AssertCalled(t, "parallel2")
	tester.AssertCalled(t, "parallel3")

	// Check results
	r1 := Get[testerOutput](state, "p1")
	if r1.Result != "mock-p1" {
		t.Errorf("p1 result: got %q, want %q", r1.Result, "mock-p1")
	}

	r2 := Get[testerOutput](state, "p2")
	if r2.Result != "mock-p2" {
		t.Errorf("p2 result: got %q, want %q", r2.Result, "mock-p2")
	}

	r3 := Get[testerOutput](state, "p3")
	if r3.Result != "mock-p3" {
		t.Errorf("p3 result: got %q, want %q", r3.Result, "mock-p3")
	}
}

func TestFlowTester_CallTracking(t *testing.T) {
	t.Parallel()

	node := NewNode("tracked", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{Value: "call1"})

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester().
		Mock("tracked", func(in testerInput) (testerOutput, error) {
			return testerOutput{Result: in.Value}, nil
		})

	// Run multiple times
	for i := 0; i < 3; i++ {
		_, err := tester.Run(flow, FlowInput{})
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", i, err)
		}
	}

	if tester.CallCount("tracked") != 3 {
		t.Errorf("call count: got %d, want 3", tester.CallCount("tracked"))
	}

	if !tester.WasCalled("tracked") {
		t.Error("WasCalled should return true")
	}

	if tester.WasCalled("nonexistent") {
		t.Error("WasCalled should return false for unmocked node")
	}
}

func TestFlowTester_CallArgs(t *testing.T) {
	t.Parallel()

	node := NewNode("with-args", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{Value: "test-value"})

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester().
		Mock("with-args", func(in testerInput) (testerOutput, error) {
			return testerOutput{}, nil
		})

	_, err := tester.Run(flow, FlowInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := tester.CallArgs("with-args")
	if len(args) != 1 {
		t.Fatalf("expected 1 call arg, got %d", len(args))
	}

	arg := args[0].(testerInput)
	if arg.Value != "test-value" {
		t.Errorf("call arg value: got %q, want %q", arg.Value, "test-value")
	}

	lastArg := tester.LastCallArg("with-args").(testerInput)
	if lastArg.Value != "test-value" {
		t.Errorf("last call arg value: got %q, want %q", lastArg.Value, "test-value")
	}
}

func TestFlowTester_Reset(t *testing.T) {
	t.Parallel()

	node := NewNode("resettable", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{})

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester().
		Mock("resettable", func(in testerInput) (testerOutput, error) {
			return testerOutput{}, nil
		})

	// Run once
	_, _ = tester.Run(flow, FlowInput{})
	if tester.CallCount("resettable") != 1 {
		t.Errorf("before reset: got %d calls, want 1", tester.CallCount("resettable"))
	}

	// Reset call tracking
	tester.Reset()
	if tester.CallCount("resettable") != 0 {
		t.Errorf("after reset: got %d calls, want 0", tester.CallCount("resettable"))
	}

	// Mock should still be registered
	_, err := tester.Run(flow, FlowInput{})
	if err != nil {
		t.Fatalf("after reset run: unexpected error: %v", err)
	}
}

func TestFlowTester_ResetAll(t *testing.T) {
	t.Parallel()

	node := NewNode("full-reset", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{})

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester().
		Mock("full-reset", func(in testerInput) (testerOutput, error) {
			return testerOutput{}, nil
		})

	// Run once
	_, _ = tester.Run(flow, FlowInput{})

	// Reset everything
	tester.ResetAll()

	// Mock should no longer be registered
	_, err := tester.Run(flow, FlowInput{})
	if err == nil {
		t.Fatal("expected error after ResetAll, mock should be gone")
	}
}

func TestFlowTester_ContextCancellation(t *testing.T) {
	t.Parallel()

	node := NewNode("cancellable", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{})

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester().
		Mock("cancellable", func(in testerInput) (testerOutput, error) {
			return testerOutput{}, nil
		})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := tester.RunWithContext(ctx, flow, FlowInput{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestFlowTester_Assertions(t *testing.T) {
	t.Parallel()

	node1 := NewNode("called-node", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{})

	node2 := NewNode("also-called", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{})

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node1).
		Then(node2).
		Build()

	tester := NewFlowTester().
		Mock("called-node", func(in testerInput) (testerOutput, error) {
			return testerOutput{}, nil
		}).
		Mock("also-called", func(in testerInput) (testerOutput, error) {
			return testerOutput{}, nil
		})

	_, _ = tester.Run(flow, FlowInput{})

	// These should not produce errors
	tester.AssertCalled(t, "called-node")
	tester.AssertCalled(t, "also-called")
	tester.AssertCallCount(t, "called-node", 1)
	tester.AssertCallCount(t, "also-called", 1)
	tester.AssertNotCalled(t, "never-called")
}

func TestFlowTester_OutputKey(t *testing.T) {
	t.Parallel()

	// Node with custom output key
	node := NewNode("original-name", func(ctx context.Context, in testerInput) (testerOutput, error) {
		return testerOutput{}, nil
	}, testerInput{}).As("custom-key")

	flow := NewFlow("test-flow").
		TriggeredBy(Manual("test")).
		Then(node).
		Build()

	tester := NewFlowTester().
		Mock("original-name", func(in testerInput) (testerOutput, error) {
			return testerOutput{Result: "custom"}, nil
		})

	state, err := tester.Run(flow, FlowInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result should be stored under custom key
	result := Get[testerOutput](state, "custom-key")
	if result.Result != "custom" {
		t.Errorf("result: got %q, want %q", result.Result, "custom")
	}
}

// mockT implements TestingT for testing assertions
type mockT struct {
	errors []string
}

func (m *mockT) Helper() {}

func (m *mockT) Errorf(format string, args ...any) {
	m.errors = append(m.errors, fmt.Sprintf(format, args...))
}

func TestFlowTester_AssertionFailures(t *testing.T) {
	t.Parallel()

	tester := NewFlowTester()

	tests := []struct {
		name      string
		assert    func(tb TestingT)
		wantError bool
	}{
		{
			name: "AssertCalled on uncalled node",
			assert: func(tb TestingT) {
				tester.AssertCalled(tb, "never-called")
			},
			wantError: true,
		},
		{
			name: "AssertNotCalled on called node",
			assert: func(tb TestingT) {
				tester.mu.Lock()
				tester.callCounts["was-called"] = 1
				tester.mu.Unlock()
				tester.AssertNotCalled(tb, "was-called")
			},
			wantError: true,
		},
		{
			name: "AssertCallCount with wrong count",
			assert: func(tb TestingT) {
				tester.mu.Lock()
				tester.callCounts["some-node"] = 3
				tester.mu.Unlock()
				tester.AssertCallCount(tb, "some-node", 5)
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTB := &mockT{}
			tt.assert(mockTB)

			if tt.wantError && len(mockTB.errors) == 0 {
				t.Error("expected assertion error, got none")
			}
			if !tt.wantError && len(mockTB.errors) > 0 {
				t.Errorf("unexpected assertion error: %v", mockTB.errors)
			}
		})
	}
}
