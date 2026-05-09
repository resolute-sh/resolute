package core

import (
	"context"
	"testing"

	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// TestFlowExecutor_LoopStepRuns verifies the Flow executor dispatches loop
// steps through executeLoopStep. The loop runs end-to-end and the final
// state is captured via the FinalState closure.
func TestFlowExecutor_LoopStepRuns(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	var capturedFinal counterState

	noop := func(_ context.Context, _ struct{}) (struct{}, error) {
		return struct{}{}, nil
	}
	noopStep := NewNode("noop", noop, struct{}{}).asStep()

	wf := func(ctx workflow.Context) error {
		// given
		b := NewFlow("loop-test").TriggeredBy(Manual("test"))
		ThenLoop[counterState](b, "loop-step",
			StateKey[counterState]("counter.state"),
			Steps[counterState](noopStep),
			AfterIteration[counterState](func(s counterState, _ FlowStateReader) counterState {
				s.N++
				return s
			}),
			While[counterState](func(s counterState) bool { return s.N < 4 }),
			MaxIterations[counterState](100),
			InitialState[counterState](func(_ *FlowState) counterState {
				return counterState{N: 0}
			}),
			FinalState[counterState](func(_ *FlowState, s counterState) {
				capturedFinal = s
			}),
		)
		flow := b.Build()

		// when
		return flow.Execute(ctx, FlowInput{})
	}

	env.RegisterWorkflow(wf)
	env.RegisterActivity(noop)
	env.ExecuteWorkflow(wf)

	// then
	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow not completed")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	if capturedFinal.N != 4 {
		t.Errorf("final state N = %d, want 4", capturedFinal.N)
	}
}
