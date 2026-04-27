package core

import (
	"testing"

	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// counterState is the test-only typed state threaded through Loop unit tests.
type counterState struct{ N int }

// counterBody implements LoopBody[counterState] for unit tests by delegating
// each iteration to a synchronous closure. Production loops would use a Body
// that calls workflow.ExecuteActivity; tests run the closure directly so the
// loop's control flow can be exercised without a worker.
type counterBody struct {
	fn func(counterState) counterState
}

func (b *counterBody) Execute(_ workflow.Context, in counterState) (counterState, error) {
	return b.fn(in), nil
}

// flowStateKeyCounter is the *FlowState key the test loops use to read and
// write the typed counterState.
const flowStateKeyCounter = "test_counter"

func TestTypedLoopRunner_HappyPath(t *testing.T) {
	tests := []struct {
		name          string
		initial       counterState
		bodyFn        func(counterState) counterState
		whileFn       func(counterState) bool
		maxIterations int
		// cancelAfterN, when > 0, wraps bodyFn so the loop's context is
		// cancelled immediately after the Nth body completion. 0 disables
		// the wrapper, leaving bodyFn untouched.
		cancelAfterN int
		wantFinal    counterState
		wantReason   LoopExitReason
		wantErr      bool
	}{
		{
			name:          "stops_when_condition_false",
			initial:       counterState{N: 0},
			bodyFn:        func(s counterState) counterState { s.N++; return s },
			whileFn:       func(s counterState) bool { return s.N < 3 },
			maxIterations: 100,
			wantFinal:     counterState{N: 3},
			wantReason:    LoopExitCondition,
			wantErr:       false,
		},
		{
			name:          "max_iterations_hit",
			initial:       counterState{N: 0},
			bodyFn:        func(s counterState) counterState { s.N++; return s },
			whileFn:       func(s counterState) bool { return true },
			maxIterations: 5,
			wantFinal:     counterState{N: 5},
			wantReason:    LoopExitMaxIterations,
			wantErr:       false,
		},
		{
			name:          "max_iterations_zero_never_runs_body",
			initial:       counterState{N: 0},
			bodyFn:        func(s counterState) counterState { s.N++; return s },
			whileFn:       func(s counterState) bool { return true },
			maxIterations: 0,
			wantFinal:     counterState{N: 0},
			wantReason:    LoopExitMaxIterations,
			wantErr:       false,
		},
		{
			name:          "cancellation_mid_loop",
			initial:       counterState{N: 0},
			bodyFn:        func(s counterState) counterState { s.N++; return s },
			whileFn:       func(s counterState) bool { return true },
			maxIterations: 100,
			cancelAfterN:  2,
			wantFinal:     counterState{N: 2},
			wantReason:    LoopExitCancelled,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ts := &testsuite.WorkflowTestSuite{}
			env := ts.NewTestWorkflowEnvironment()

			initial := tt.initial
			var got counterState
			var gotReason LoopExitReason
			var gotErr error

			wf := func(ctx workflow.Context) error {
				loopCtx, cancel := workflow.WithCancel(ctx)

				body := tt.bodyFn
				if tt.cancelAfterN > 0 {
					inner := body
					body = func(s counterState) counterState {
						out := inner(s)
						if out.N == tt.cancelAfterN {
							cancel()
						}
						return out
					}
				}

				fs := NewFlowState(FlowInput{})
				fs.SetResult(flowStateKeyCounter, initial)

				runner := &typedLoopRunner[counterState]{
					cfg: &loopConfig[counterState]{
						body:          &counterBody{fn: body},
						while:         tt.whileFn,
						maxIterations: tt.maxIterations,
						initialFromFS: func(fs *FlowState) counterState {
							return Get[counterState](fs, flowStateKeyCounter)
						},
						finalToFS: func(fs *FlowState, s counterState) {
							fs.SetResult(flowStateKeyCounter, s)
						},
					},
				}
				gotReason, gotErr = runner.runLoop(loopCtx, fs)
				got = Get[counterState](fs, flowStateKeyCounter)
				return nil
			}

			env.RegisterWorkflow(wf)
			env.ExecuteWorkflow(wf)

			if !env.IsWorkflowCompleted() {
				t.Fatalf("workflow not completed")
			}
			if err := env.GetWorkflowError(); err != nil {
				t.Fatalf("workflow error: %v", err)
			}
			if tt.wantErr {
				if gotErr == nil {
					t.Errorf("runLoop returned nil error, want non-nil")
				}
			} else if gotErr != nil {
				t.Errorf("runLoop error = %v, want nil", gotErr)
			}
			if got.N != tt.wantFinal.N {
				t.Errorf("final state N = %d, want %d", got.N, tt.wantFinal.N)
			}
			if gotReason != tt.wantReason {
				t.Errorf("exit reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

// TestTypedLoopRunner_IterationHook verifies the IterationHook callback fires
// twice per iteration ("started" before the body, "completed" after) with
// monotonically increasing N starting at 1.
func TestTypedLoopRunner_IterationHook(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	var events []IterationEvent

	wf := func(ctx workflow.Context) error {
		fs := NewFlowState(FlowInput{})
		fs.SetResult(flowStateKeyCounter, counterState{N: 0})

		runner := &typedLoopRunner[counterState]{
			cfg: &loopConfig[counterState]{
				body: &counterBody{fn: func(s counterState) counterState {
					s.N++
					return s
				}},
				while:         func(s counterState) bool { return s.N < 3 },
				maxIterations: 100,
				iterHook: func(e IterationEvent) {
					events = append(events, e)
				},
				initialFromFS: func(fs *FlowState) counterState {
					return Get[counterState](fs, flowStateKeyCounter)
				},
				finalToFS: func(fs *FlowState, s counterState) {
					fs.SetResult(flowStateKeyCounter, s)
				},
			},
		}
		_, _ = runner.runLoop(ctx, fs)
		return nil
	}

	env.RegisterWorkflow(wf)
	env.ExecuteWorkflow(wf)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow not completed")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	if len(events) != 6 {
		t.Fatalf("event count = %d, want 6: %+v", len(events), events)
	}
	expected := []IterationEvent{
		{Phase: "started", N: 1},
		{Phase: "completed", N: 1},
		{Phase: "started", N: 2},
		{Phase: "completed", N: 2},
		{Phase: "started", N: 3},
		{Phase: "completed", N: 3},
	}
	for i, want := range expected {
		if events[i] != want {
			t.Errorf("event[%d] = %+v, want %+v", i, events[i], want)
		}
	}
}
