package core

import (
	"fmt"

	"go.temporal.io/sdk/workflow"
)

// ItemActivity is what the user provides as the per-item activity. Typed in
// I/O so no `any` appears at the user boundary. Production implementations
// invoke workflow.ExecuteActivity; tests can run synchronously.
type ItemActivity[I, O any] interface {
	Execute(ctx workflow.Context, in I) (O, error)
}

// ParallelEachOption configures a ParallelEach step. Typed in I and O.
type ParallelEachOption[I, O any] func(*parallelEachConfig[I, O])

type parallelEachConfig[I, O any] struct {
	from           func(*FlowState) []I
	body           ItemActivity[I, O]
	merge          func(*FlowState, []O)
	maxConcurrency int
	onItemError    ItemErrorMode
	perItemRetry   *RetryPolicyOverride
}

// parallelEachRunner is the unexported Step-level seam. Implementations are
// typed (typedParallelEachRunner[I, O]) so the Step holds an interface, not
// an `any` field.
type parallelEachRunner interface {
	runParallelEach(ctx workflow.Context, fs *FlowState) error
}

// typedParallelEachRunner is the concrete generic implementation behind
// parallelEachRunner. It is constructed by ParallelEach[I, O] and stored
// inside a parallelEachStep via the parallelEachRunner interface, keeping I
// and O out of the non-generic Step type.
type typedParallelEachRunner[I, O any] struct {
	cfg *parallelEachConfig[I, O]
}

func (r *typedParallelEachRunner[I, O]) runParallelEach(ctx workflow.Context, fs *FlowState) error {
	if r.cfg.from == nil {
		return fmt.Errorf("flow.ParallelEach: from option is required")
	}
	if r.cfg.body == nil {
		return fmt.Errorf("flow.ParallelEach: body option is required")
	}
	if r.cfg.merge == nil {
		return fmt.Errorf("flow.ParallelEach: merge option is required")
	}

	items := r.cfg.from(fs)
	if len(items) == 0 {
		r.cfg.merge(fs, nil)
		return nil
	}

	n := len(items)
	results := make([]O, n)
	errs := make([]error, n)

	doneCh := workflow.NewBufferedChannel(ctx, n)

	var sem workflow.Channel
	if r.cfg.maxConcurrency > 0 {
		sem = workflow.NewBufferedChannel(ctx, r.cfg.maxConcurrency)
	}

	failFastCh := workflow.NewBufferedChannel(ctx, 1)
	var failFastTriggered bool

	checkFailFast := func(c workflow.Context) bool {
		sel := workflow.NewSelector(c)
		triggered := false
		sel.AddReceive(failFastCh, func(workflow.ReceiveChannel, bool) { triggered = true })
		sel.AddDefault(func() {})
		sel.Select(c)
		if triggered {
			failFastCh.SendAsync(struct{}{})
		}
		return triggered
	}

	for i := 0; i < n; i++ {
		idx := i
		workflow.Go(ctx, func(gCtx workflow.Context) {
			if sem != nil {
				sem.Send(gCtx, struct{}{})
				defer func() {
					var slot struct{}
					sem.Receive(gCtx, &slot)
				}()
			}

			if checkFailFast(gCtx) || gCtx.Err() != nil {
				doneCh.Send(gCtx, idx)
				return
			}

			out, err := r.cfg.body.Execute(gCtx, items[idx])
			results[idx] = out
			errs[idx] = err

			if err != nil && r.cfg.onItemError == ItemErrorFailFast && !failFastTriggered {
				failFastTriggered = true
				failFastCh.SendAsync(struct{}{})
			}

			doneCh.Send(gCtx, idx)
		})
	}

	for i := 0; i < n; i++ {
		var idx int
		doneCh.Receive(ctx, &idx)
	}

	if r.cfg.onItemError == ItemErrorFailFast {
		for _, e := range errs {
			if e != nil {
				return e
			}
		}
	}

	r.cfg.merge(fs, results)
	return nil
}

// parallelEachStep is the non-generic carrier stored on Step for ParallelEach
// execution. It dispatches through the parallelEachRunner interface so the
// Step type stays free of generic parameters.
type parallelEachStep struct {
	runner parallelEachRunner
}

// ParallelEach returns a Step that fans out one activity per element of a
// runtime-derived list, awaits all, and folds the results back into
// *FlowState via the user-provided merge closure.
//
// Concretely: From(fs) -> []I; for each I, body.Execute(ctx, I) -> O; then
// merge(fs, []O) writes the aggregated results back. Activities are scheduled
// via workflow.Go; concurrency is bounded by MaxConcurrency when set.
//
// Determinism contract: From and Merge run in workflow code and must be
// deterministic functions of *FlowState. The body activity has no
// determinism constraint.
//
// History event emission (ParallelEachStarted/ParallelEachCompleted markers)
// is planned for a future task; the current implementation only surfaces
// item errors via the activity error chain.
func ParallelEach[I, O any](opts ...ParallelEachOption[I, O]) Step {
	cfg := &parallelEachConfig[I, O]{
		onItemError: ItemErrorContinue,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return Step{
		name:         "parallel-each",
		parallelEach: &parallelEachStep{runner: &typedParallelEachRunner[I, O]{cfg: cfg}},
	}
}

// ThenParallelEach adds a ParallelEach step to the builder under the given
// name. It is a generic free function because Go does not allow generic
// methods on non-generic types; call sites read flow.ThenParallelEach(b,
// "name", ...).
func ThenParallelEach[I, O any](b *FlowBuilder, name string, opts ...ParallelEachOption[I, O]) *FlowBuilder {
	s := ParallelEach[I, O](opts...)
	s.name = name
	b.flow.steps = append(b.flow.steps, s)
	return b
}

// From sets the per-item input deriver. Required.
//
// The closure must be deterministic over *FlowState — it runs in workflow
// code.
func From[I, O any](fn func(*FlowState) []I) ParallelEachOption[I, O] {
	return func(c *parallelEachConfig[I, O]) { c.from = fn }
}

// ParallelBody sets the per-item activity. Required.
func ParallelBody[I, O any](a ItemActivity[I, O]) ParallelEachOption[I, O] {
	return func(c *parallelEachConfig[I, O]) { c.body = a }
}

// Merge sets the result folder. Required. Receives *FlowState and the
// ordered []O (length == len([]I)). On ContinueOnError, failed items
// produce zero-value O entries.
//
// The closure must be deterministic over its inputs — it runs in workflow
// code.
func Merge[I, O any](fn func(*FlowState, []O)) ParallelEachOption[I, O] {
	return func(c *parallelEachConfig[I, O]) { c.merge = fn }
}

// MaxConcurrency caps simultaneous activities. Default unbounded (0).
//
// Useful for rate-limited downstream services.
func MaxConcurrency[I, O any](n int) ParallelEachOption[I, O] {
	return func(c *parallelEachConfig[I, O]) { c.maxConcurrency = n }
}

// PerItemRetry overrides the activity-default retry policy for each item.
//
// Reserved; consumed when activity-options plumbing lands in a future task.
func PerItemRetry[I, O any](p RetryPolicyOverride) ParallelEachOption[I, O] {
	return func(c *parallelEachConfig[I, O]) { c.perItemRetry = &p }
}

// OnItemError controls failed-item handling. Default ItemErrorContinue.
func OnItemError[I, O any](m ItemErrorMode) ParallelEachOption[I, O] {
	return func(c *parallelEachConfig[I, O]) { c.onItemError = m }
}

// ItemErrorMode controls how ParallelEach handles per-item failures.
type ItemErrorMode int

const (
	// ItemErrorContinue: failed items get a zero-value O; Merge sees the
	// full ordered result list. Suitable for LLM tool dispatch where partial
	// failures are normal.
	ItemErrorContinue ItemErrorMode = iota
	// ItemErrorFailFast: first failure cancels remaining items and the
	// error propagates from runParallelEach.
	ItemErrorFailFast
)

// RetryPolicyOverride configures per-item retry behavior. Reserved for a
// future task; the current runner does not consume it.
type RetryPolicyOverride struct {
	InitialInterval    string // duration string, parsed when wired
	BackoffCoefficient float64
	MaxAttempts        int
}
