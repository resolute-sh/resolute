package core

import (
	"context"
	"testing"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// counterState is the test-only typed state threaded through Loop unit tests.
type counterState struct{ N int }

const flowStateKeyCounter = "test_counter"

// loopTestNoopActivity is registered with the WorkflowTestSuite by name so
// every loop body Step can reference it without binding to a unique closure.
// Without name registration, each test creates a unique func value that
// Temporal cannot match to a registered activity.
const loopTestNoopActivityName = "loop-test-noop"

func loopTestNoop(_ context.Context, _ struct{}) (struct{}, error) {
	return struct{}{}, nil
}

// noopBodyStep returns a Loop body Step that invokes the named noop activity.
// Tests register loopTestNoop with the activity registry under
// loopTestNoopActivityName before executing the workflow.
func noopBodyStep() Step {
	node := NewNodeByName[struct{}, struct{}]("noop", loopTestNoopActivityName, struct{}{})
	return AsStep(node)
}

// runLoopFlow constructs a Flow with a single Loop step (built via the given
// options) and executes it inside a Temporal WorkflowTestSuite. Returns the
// captured final counterState (via FinalState callback).
func runLoopFlow(t *testing.T, opts ...LoopOption[counterState]) counterState {
	t.Helper()

	var capturedFinal counterState
	wf := func(ctx workflow.Context) error {
		b := NewFlow("loop-test").TriggeredBy(Manual("test"))
		allOpts := append([]LoopOption[counterState]{
			FinalState[counterState](func(_ *FlowState, s counterState) {
				capturedFinal = s
			}),
		}, opts...)
		ThenLoop[counterState](b, "loop-step", allOpts...)
		return b.Build().Execute(ctx, FlowInput{})
	}

	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(wf)
	env.RegisterActivityWithOptions(loopTestNoop, activity.RegisterOptions{Name: loopTestNoopActivityName})
	env.ExecuteWorkflow(wf)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow not completed")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	return capturedFinal
}

func TestLoop_StopsWhenConditionFalse(t *testing.T) {
	final := runLoopFlow(t,
		StateKey[counterState](flowStateKeyCounter),
		InitialState[counterState](func(_ *FlowState) counterState { return counterState{N: 0} }),
		Steps[counterState](noopBodyStep()),
		AfterIteration[counterState](func(s counterState, _ FlowStateReader) counterState {
			s.N++
			return s
		}),
		While[counterState](func(s counterState) bool { return s.N < 3 }),
		MaxIterations[counterState](100),
	)
	if final.N != 3 {
		t.Fatalf("counter = %d, want 3", final.N)
	}
}

func TestLoop_MaxIterationsHit(t *testing.T) {
	final := runLoopFlow(t,
		StateKey[counterState](flowStateKeyCounter),
		InitialState[counterState](func(_ *FlowState) counterState { return counterState{N: 0} }),
		Steps[counterState](noopBodyStep()),
		AfterIteration[counterState](func(s counterState, _ FlowStateReader) counterState {
			s.N++
			return s
		}),
		While[counterState](func(s counterState) bool { return true }),
		MaxIterations[counterState](5),
	)
	if final.N != 5 {
		t.Fatalf("counter = %d, want 5", final.N)
	}
}

func TestLoop_MaxIterationsZeroNeverRunsBody(t *testing.T) {
	final := runLoopFlow(t,
		StateKey[counterState](flowStateKeyCounter),
		InitialState[counterState](func(_ *FlowState) counterState { return counterState{N: 0} }),
		Steps[counterState](noopBodyStep()),
		AfterIteration[counterState](func(s counterState, _ FlowStateReader) counterState {
			s.N++
			return s
		}),
		While[counterState](func(s counterState) bool { return true }),
		MaxIterations[counterState](0),
	)
	if final.N != 0 {
		t.Fatalf("counter = %d, want 0", final.N)
	}
}

// TestLoop_BeforeIterationShortCircuit verifies that returning skipBody=true
// from BeforeIteration skips the Steps body for that iteration but still runs
// AfterIteration. The skip predicate fires on the iteration where N=1, so
// the body activity should run on iterations where N=0 and N=2 but NOT on
// the iteration where N=1.
func TestLoop_BeforeIterationShortCircuit(t *testing.T) {
	const skipNoopName = "loop-test-noop-skip"
	var noopCalls int
	noopFn := func(_ context.Context, _ struct{}) (struct{}, error) {
		noopCalls++
		return struct{}{}, nil
	}
	noopStep := AsStep(NewNodeByName[struct{}, struct{}]("noop-skip", skipNoopName, struct{}{}))

	wf := func(ctx workflow.Context) error {
		b := NewFlow("skip-test").TriggeredBy(Manual("test"))
		ThenLoop[counterState](b, "skip-loop",
			StateKey[counterState](flowStateKeyCounter),
			InitialState[counterState](func(_ *FlowState) counterState { return counterState{N: 0} }),
			Steps[counterState](noopStep),
			BeforeIteration[counterState](func(s counterState, _ *FlowState) (counterState, bool) {
				return s, s.N == 1
			}),
			AfterIteration[counterState](func(s counterState, _ FlowStateReader) counterState {
				s.N++
				return s
			}),
			While[counterState](func(s counterState) bool { return s.N < 3 }),
			MaxIterations[counterState](10),
		)
		return b.Build().Execute(ctx, FlowInput{})
	}

	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(wf)
	env.RegisterActivityWithOptions(noopFn, activity.RegisterOptions{Name: skipNoopName})
	env.ExecuteWorkflow(wf)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow not completed")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	// Iteration 1: N=0, body runs (1) → AfterIteration → N=1
	// Iteration 2: N=1, body skipped     → AfterIteration → N=2
	// Iteration 3: N=2, body runs (2)    → AfterIteration → N=3 → While exits
	if noopCalls != 2 {
		t.Fatalf("noop activity ran %d times, want 2 (iteration with N=1 should have been skipped)", noopCalls)
	}
}
