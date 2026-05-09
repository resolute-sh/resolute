package core

import (
	"go.temporal.io/sdk/workflow"
)

// LoopBody is the per-iteration step a caller supplies to flow.Loop. It is
// typed in S so no `any` appears at the user boundary; the body receives the
// current loop state and returns the next iteration's state (or an error).
type LoopBody[S any] interface {
	Execute(ctx workflow.Context, in S) (S, error)
}

// LoopBodyWithState is a LoopBody that also receives *FlowState.
type LoopBodyWithState[S any] interface {
	LoopBody[S]
	ExecuteWithState(ctx workflow.Context, state S, fs *FlowState) (S, error)
}

// LoopOption configures a Loop. Apply options via the Loop or ThenLoop
// constructors.
type LoopOption[S any] func(*loopConfig[S])

// loopConfig is the internal configuration assembled from LoopOption values.
type loopConfig[S any] struct {
	body          LoopBody[S]
	while         func(S) bool
	maxIterations int
	iterHook      func(IterationEvent)
	canPolicy     ContinueAsNewPolicy // reserved; consumed when CAN emission lands in a future task
	initialFromFS func(*FlowState) S
	finalToFS     func(*FlowState, S)
}

// loopRunner is the unexported Step-level seam. Implementations are typed
// (typedLoopRunner[S]) so the Step holds an interface, not an `any` field.
type loopRunner interface {
	runLoop(ctx workflow.Context, fs *FlowState) (LoopExitReason, error)
}

// typedLoopRunner is the concrete generic implementation behind loopRunner.
// It is constructed by Loop[S] and stored inside a loopStep via the
// loopRunner interface, keeping S out of the non-generic Step type.
type typedLoopRunner[S any] struct {
	cfg *loopConfig[S]
}

func (r *typedLoopRunner[S]) runLoop(ctx workflow.Context, fs *FlowState) (LoopExitReason, error) {
	state := r.cfg.initialFromFS(fs)
	reason := LoopExitMaxIterations
	for i := 0; i < r.cfg.maxIterations; i++ {
		if ctx.Err() != nil {
			r.cfg.finalToFS(fs, state)
			return LoopExitCancelled, ctx.Err()
		}
		if r.cfg.iterHook != nil {
			r.cfg.iterHook(IterationEvent{Phase: "started", N: i + 1})
		}

		var next S
		var err error
		if bodyWS, ok := r.cfg.body.(LoopBodyWithState[S]); ok {
			next, err = bodyWS.ExecuteWithState(ctx, state, fs)
		} else {
			next, err = r.cfg.body.Execute(ctx, state)
		}
		if err != nil {
			r.cfg.finalToFS(fs, state)
			return reason, err
		}
		state = next

		if r.cfg.iterHook != nil {
			r.cfg.iterHook(IterationEvent{Phase: "completed", N: i + 1})
		}
		if ctx.Err() != nil {
			r.cfg.finalToFS(fs, state)
			return LoopExitCancelled, ctx.Err()
		}
		if !r.cfg.while(state) {
			reason = LoopExitCondition
			break
		}
	}
	r.cfg.finalToFS(fs, state)
	return reason, nil
}

// loopStep is the non-generic carrier stored on Step for loop execution.
// It dispatches through the loopRunner interface so the Step type stays
// free of generic parameters.
type loopStep struct {
	runner loopRunner
}

// Loop returns a Step that executes Body repeatedly while While returns true,
// capped by MaxIterations. State is threaded between iterations as the typed
// value S; each iteration body output becomes the next iteration's input. The
// While predicate runs after each body completion.
//
// State enters and leaves the loop via *FlowState — InitialState reads the
// seed S, FinalState writes the final S back. Inter-iteration threading uses
// a workflow-local Go variable to avoid the *FlowState lock contention path.
//
// Determinism contract: Body activities are unconstrained, but the While
// predicate, MaxIterations value, IterationHook callback, InitialState, and
// FinalState closures all run in workflow code and must be deterministic
// functions of their inputs (no time.Now, no rand, no map iteration order,
// no I/O).
//
// History event emission (IterationStarted/IterationCompleted/LoopExited
// markers) is planned for a future task; the current implementation only
// surfaces termination via the LoopExitReason returned through the Step
// executor.
func Loop[S any](opts ...LoopOption[S]) Step {
	cfg := &loopConfig[S]{
		maxIterations: 1000,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return Step{
		name: "loop",
		loop: &loopStep{runner: &typedLoopRunner[S]{cfg: cfg}},
	}
}

// ThenLoop adds a Loop step to the builder under the given name. It is a
// generic free function because Go does not allow generic methods on
// non-generic types; call sites read flow.ThenLoop(b, "name", ...).
func ThenLoop[S any](b *FlowBuilder, name string, opts ...LoopOption[S]) *FlowBuilder {
	s := Loop[S](opts...)
	s.name = name
	b.flow.steps = append(b.flow.steps, s)
	return b
}

// Body sets the iteration body. The body receives the current S and returns
// the next S; iteration continues until While returns false or MaxIterations
// is reached.
func Body[S any](b LoopBody[S]) LoopOption[S] {
	return func(c *loopConfig[S]) { c.body = b }
}

// BodyWithState sets a loop body that receives both S and *FlowState.
func BodyWithState[S any](b LoopBodyWithState[S]) LoopOption[S] {
	return func(c *loopConfig[S]) { c.body = b }
}

// While sets the continuation predicate. The runner evaluates While after
// each body completion; when it returns false (or MaxIterations is reached
// or the workflow is cancelled), the loop exits.
//
// The predicate MUST be deterministic over S — no time.Now, no rand, no map
// iteration order, no external I/O, no captured state that varies across
// replays. Violations cause Temporal replay failures (NonDeterministicError);
// the violation typically does NOT surface on first execution, only on
// worker restart or query replay. Layer 2 workflow replay tests in
// resolute/core (Task 13) verify deterministic predicates pass replay.
func While[S any](cond func(S) bool) LoopOption[S] {
	return func(c *loopConfig[S]) { c.while = cond }
}

// InitialState supplies the function that extracts the loop's seed S from the
// flow's *FlowState. Required.
func InitialState[S any](extract func(*FlowState) S) LoopOption[S] {
	return func(c *loopConfig[S]) { c.initialFromFS = extract }
}

// FinalState supplies the function that writes the loop's final S back into
// the flow's *FlowState. Required.
func FinalState[S any](inject func(*FlowState, S)) LoopOption[S] {
	return func(c *loopConfig[S]) { c.finalToFS = inject }
}

// MaxIterations sets the hard ceiling on iteration count. Default 1000.
func MaxIterations[S any](n int) LoopOption[S] {
	return func(c *loopConfig[S]) { c.maxIterations = n }
}

// IterationHook installs a callback invoked around each iteration. The hook
// runs in workflow code and must be deterministic.
func IterationHook[S any](h func(IterationEvent)) LoopOption[S] {
	return func(c *loopConfig[S]) { c.iterHook = h }
}

// ContinueAsNewAfter installs a continue-as-new policy. Default: never.
func ContinueAsNewAfter[S any](p ContinueAsNewPolicy) LoopOption[S] {
	return func(c *loopConfig[S]) { c.canPolicy = p }
}

// IterationEvent is the payload delivered to an IterationHook. Phase is one
// of "started" or "completed"; N is the 1-indexed iteration number.
type IterationEvent struct {
	Phase string
	N     int
}

// ContinueAsNewPolicy controls when the loop emits ContinueAsNew. Both
// fields default to 0, meaning "never emit ContinueAsNew."
type ContinueAsNewPolicy struct {
	AfterIterations   int   // 0 = never
	AfterHistoryBytes int64 // 0 = never (best-effort, checked between iterations)
}

// LoopExitReason identifies why a Loop terminated. It is recorded on the
// final LoopExited marker in workflow history.
type LoopExitReason string

const (
	// LoopExitCondition indicates the While predicate returned false.
	LoopExitCondition LoopExitReason = "condition"
	// LoopExitMaxIterations indicates MaxIterations was reached.
	LoopExitMaxIterations LoopExitReason = "max_iterations"
	// LoopExitCancelled indicates the workflow context was cancelled mid-loop.
	LoopExitCancelled LoopExitReason = "cancelled"
	// LoopExitContinueAsNew indicates the loop emitted ContinueAsNew per
	// ContinueAsNewPolicy. Reserved for a future task; the current runLoop
	// never returns this value.
	LoopExitContinueAsNew LoopExitReason = "continue_as_new"
)
