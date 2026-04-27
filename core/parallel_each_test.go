package core

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// stringBody is a deterministic-from-input ItemActivity[int, string] for
// ParallelEach unit tests.
type stringBody struct {
	fn func(int) (string, error)
}

func (b *stringBody) Execute(_ workflow.Context, in int) (string, error) {
	return b.fn(in)
}

// flowStateKeyItems / flowStateKeyResults are the *FlowState keys used by the
// ParallelEach unit tests to read inputs and stash merged outputs.
const (
	flowStateKeyItems   = "test_pe_items"
	flowStateKeyResults = "test_pe_results"
)

// TestTypedParallelEachRunner_BasicShapes covers the empty-list, single-item,
// and N-item paths. The N-item case verifies results are returned in input
// order even when bodies complete out of order.
func TestTypedParallelEachRunner_BasicShapes(t *testing.T) {
	tests := []struct {
		name        string
		items       []int
		bodyFn      func(int) (string, error)
		wantResults []string
	}{
		{
			name:        "empty_list",
			items:       nil,
			bodyFn:      func(i int) (string, error) { return fmt.Sprintf("v%d", i), nil },
			wantResults: nil,
		},
		{
			name:        "single_item",
			items:       []int{42},
			bodyFn:      func(i int) (string, error) { return fmt.Sprintf("v%d", i), nil },
			wantResults: []string{"v42"},
		},
		{
			name:  "n_items_input_order_preserved",
			items: []int{1, 2, 3, 4, 5},
			bodyFn: func(i int) (string, error) {
				// Sleep proportional to (10 - i) so completion order
				// diverges from input order — early items take longer.
				time.Sleep(time.Duration(10-i) * time.Millisecond)
				return fmt.Sprintf("v%d", i), nil
			},
			wantResults: []string{"v1", "v2", "v3", "v4", "v5"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ts := &testsuite.WorkflowTestSuite{}
			env := ts.NewTestWorkflowEnvironment()

			var got []string

			wf := func(ctx workflow.Context) error {
				fs := NewFlowState(FlowInput{})
				fs.SetResult(flowStateKeyItems, tt.items)

				runner := &typedParallelEachRunner[int, string]{
					cfg: &parallelEachConfig[int, string]{
						from: func(fs *FlowState) []int { return Get[[]int](fs, flowStateKeyItems) },
						body: &stringBody{fn: tt.bodyFn},
						merge: func(fs *FlowState, results []string) {
							fs.SetResult(flowStateKeyResults, results)
						},
						onItemError: ItemErrorContinue,
					},
				}
				if err := runner.runParallelEach(ctx, fs); err != nil {
					return err
				}
				got = Get[[]string](fs, flowStateKeyResults)
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

			if len(got) != len(tt.wantResults) {
				t.Fatalf("len(results) = %d, want %d (got=%+v)", len(got), len(tt.wantResults), got)
			}
			for i, want := range tt.wantResults {
				if got[i] != want {
					t.Errorf("results[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

// TestTypedParallelEachRunner_MaxConcurrency verifies that with
// MaxConcurrency=2 and 8 items, peak in-flight count never exceeds 2.
func TestTypedParallelEachRunner_MaxConcurrency(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	var inFlight, peak int64

	body := &stringBody{fn: func(i int) (string, error) {
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			p := atomic.LoadInt64(&peak)
			if cur <= p || atomic.CompareAndSwapInt64(&peak, p, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
		return "ok", nil
	}}

	wf := func(ctx workflow.Context) error {
		fs := NewFlowState(FlowInput{})
		items := []int{0, 1, 2, 3, 4, 5, 6, 7}
		fs.SetResult(flowStateKeyItems, items)

		runner := &typedParallelEachRunner[int, string]{
			cfg: &parallelEachConfig[int, string]{
				from:           func(fs *FlowState) []int { return Get[[]int](fs, flowStateKeyItems) },
				body:           body,
				merge:          func(fs *FlowState, results []string) { fs.SetResult(flowStateKeyResults, results) },
				maxConcurrency: 2,
				onItemError:    ItemErrorContinue,
			},
		}
		return runner.runParallelEach(ctx, fs)
	}

	env.RegisterWorkflow(wf)
	env.ExecuteWorkflow(wf)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow not completed")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	p := atomic.LoadInt64(&peak)
	if p > 2 {
		t.Errorf("peak in-flight = %d, want <= 2 (MaxConcurrency)", p)
	}
}

// TestTypedParallelEachRunner_OnItemError covers ContinueOnError (failed
// items become zero-value O entries; merge sees full ordered list) and
// FailFast (first failure cancels remaining items, error propagates).
func TestTypedParallelEachRunner_OnItemError(t *testing.T) {
	tests := []struct {
		name        string
		items       []int
		bodyFn      func(int) (string, error)
		mode        ItemErrorMode
		wantResults []string
		wantErrSub  string // empty = expect no error
	}{
		{
			name:  "continue_on_error_failed_item_zero_value",
			items: []int{1, 2, 3},
			bodyFn: func(i int) (string, error) {
				if i == 2 {
					return "", fmt.Errorf("boom on %d", i)
				}
				return fmt.Sprintf("v%d", i), nil
			},
			mode:        ItemErrorContinue,
			wantResults: []string{"v1", "", "v3"},
			wantErrSub:  "",
		},
		{
			name:  "fail_fast_propagates_first_error",
			items: []int{1, 2, 3},
			bodyFn: func(i int) (string, error) {
				if i == 2 {
					return "", fmt.Errorf("boom on %d", i)
				}
				// Slow successful items so the failure has time to fire first.
				time.Sleep(50 * time.Millisecond)
				return fmt.Sprintf("v%d", i), nil
			},
			mode:        ItemErrorFailFast,
			wantResults: nil, // merge not called when fail-fast triggers
			wantErrSub:  "boom on 2",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ts := &testsuite.WorkflowTestSuite{}
			env := ts.NewTestWorkflowEnvironment()

			var got []string
			var runErr error

			wf := func(ctx workflow.Context) error {
				fs := NewFlowState(FlowInput{})
				fs.SetResult(flowStateKeyItems, tt.items)

				runner := &typedParallelEachRunner[int, string]{
					cfg: &parallelEachConfig[int, string]{
						from:        func(fs *FlowState) []int { return Get[[]int](fs, flowStateKeyItems) },
						body:        &stringBody{fn: tt.bodyFn},
						merge:       func(fs *FlowState, results []string) { fs.SetResult(flowStateKeyResults, results) },
						onItemError: tt.mode,
					},
				}
				runErr = runner.runParallelEach(ctx, fs)
				got = GetOr[[]string](fs, flowStateKeyResults, nil)
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

			if tt.wantErrSub == "" {
				if runErr != nil {
					t.Errorf("runner err = %v, want nil", runErr)
				}
			} else {
				if runErr == nil {
					t.Errorf("runner err = nil, want non-nil containing %q", tt.wantErrSub)
				} else if !strings.Contains(runErr.Error(), tt.wantErrSub) {
					t.Errorf("runner err = %q, want substring %q", runErr.Error(), tt.wantErrSub)
				}
			}

			if tt.wantResults == nil {
				_ = got
				return
			}
			if len(got) != len(tt.wantResults) {
				t.Fatalf("len(results) = %d, want %d (got=%+v)", len(got), len(tt.wantResults), got)
			}
			for i, want := range tt.wantResults {
				if got[i] != want {
					t.Errorf("results[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

// cancellableBody is an ItemActivity[int, string] that uses workflow.Sleep
// for cancellation cooperation. It tracks the number of items that ran to
// completion, so the test can assert cancellation interrupted some items.
type cancellableBody struct {
	completed *int64
}

func (b *cancellableBody) Execute(ctx workflow.Context, _ int) (string, error) {
	if err := workflow.Sleep(ctx, 200*time.Millisecond); err != nil {
		return "", err
	}
	atomic.AddInt64(b.completed, 1)
	return "ok", nil
}

// TestTypedParallelEachRunner_Cancellation cancels the workflow context
// shortly after fan-out starts; with MaxConcurrency=2, the post-acquire
// gCtx.Err() check skips the remaining items so most never start.
func TestTypedParallelEachRunner_Cancellation(t *testing.T) {
	ts := &testsuite.WorkflowTestSuite{}
	env := ts.NewTestWorkflowEnvironment()

	var completed int64
	body := &cancellableBody{completed: &completed}

	wf := func(ctx workflow.Context) error {
		cancelCtx, cancel := workflow.WithCancel(ctx)

		workflow.Go(cancelCtx, func(c workflow.Context) {
			_ = workflow.Sleep(c, 20*time.Millisecond)
			cancel()
		})

		fs := NewFlowState(FlowInput{})
		items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		fs.SetResult(flowStateKeyItems, items)

		runner := &typedParallelEachRunner[int, string]{
			cfg: &parallelEachConfig[int, string]{
				from:           func(fs *FlowState) []int { return Get[[]int](fs, flowStateKeyItems) },
				body:           body,
				merge:          func(fs *FlowState, results []string) { fs.SetResult(flowStateKeyResults, results) },
				maxConcurrency: 2,
				onItemError:    ItemErrorContinue,
			},
		}
		_ = runner.runParallelEach(cancelCtx, fs)
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

	c := atomic.LoadInt64(&completed)
	if c >= 10 {
		t.Errorf("completed = %d (all 10), expected cancellation to interrupt some items", c)
	}
}
