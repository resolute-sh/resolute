package core

import (
	"fmt"

	"go.temporal.io/sdk/workflow"
)

// LoopOption configures a Loop. Apply options via the Loop or ThenLoop
// constructors.
type LoopOption[S any] func(*loopConfig[S])

// loopConfig is the internal configuration assembled from LoopOption values.
type loopConfig[S any] struct {
	steps           []Step
	while           func(S) bool
	maxIterations   int
	stateKey        string
	initialFromFS   func(*FlowState) S
	finalToFS       func(*FlowState, S)
	beforeIteration func(S, *FlowState) (S, bool)
	afterIteration  func(S, FlowStateReader) S
	iterHook        func(IterationEvent)
	canPolicy       ContinueAsNewPolicy // reserved
}

// loopRunner is the unexported Step-level seam.
type loopRunner interface {
	runLoop(ctx workflow.Context, fs *FlowState, flowName string, hooks *FlowHooks) (LoopExitReason, error)
}

// typedLoopRunner is the concrete generic implementation behind loopRunner.
type typedLoopRunner[S any] struct {
	cfg *loopConfig[S]
}

func (r *typedLoopRunner[S]) runLoop(ctx workflow.Context, fs *FlowState, flowName string, hooks *FlowHooks) (LoopExitReason, error) {
	if r.cfg.stateKey == "" {
		return LoopExitMaxIterations, fmt.Errorf("Loop: StateKey option is required")
	}
	if r.cfg.initialFromFS == nil {
		return LoopExitMaxIterations, fmt.Errorf("Loop: InitialState option is required")
	}
	if r.cfg.while == nil {
		return LoopExitMaxIterations, fmt.Errorf("Loop: While option is required")
	}
	if len(r.cfg.steps) == 0 {
		return LoopExitMaxIterations, fmt.Errorf("Loop: Steps option requires at least one Step")
	}

	state := r.cfg.initialFromFS(fs)
	Set(fs, r.cfg.stateKey, state)

	reason := LoopExitMaxIterations

	for i := 0; i < r.cfg.maxIterations; i++ {
		if ctx.Err() != nil {
			r.writeFinal(fs, state)
			return LoopExitCancelled, ctx.Err()
		}
		if r.cfg.iterHook != nil {
			r.cfg.iterHook(IterationEvent{Phase: "started", N: i + 1})
		}

		// BeforeIteration: drain signals, mutate S, optionally short-circuit.
		skipBody := false
		if r.cfg.beforeIteration != nil {
			state, skipBody = r.cfg.beforeIteration(state, fs)
			Set(fs, r.cfg.stateKey, state)
		}

		if !skipBody {
			// Execute Steps body via the shared step dispatcher. Compensations
			// inside loop bodies are not collected — Loop iterations are
			// considered atomic from the parent flow's perspective.
			var ignoreCompensations []CompensationEntry
			if err := executeSteps(ctx, r.cfg.steps, fs, flowName, &ignoreCompensations, hooks); err != nil {
				r.writeFinal(fs, state)
				return reason, err
			}
		}

		// AfterIteration: read step outputs, return next S.
		if r.cfg.afterIteration != nil {
			state = r.cfg.afterIteration(state, fs)
			Set(fs, r.cfg.stateKey, state)
		}

		if r.cfg.iterHook != nil {
			r.cfg.iterHook(IterationEvent{Phase: "completed", N: i + 1})
		}
		if ctx.Err() != nil {
			r.writeFinal(fs, state)
			return LoopExitCancelled, ctx.Err()
		}
		if !r.cfg.while(state) {
			reason = LoopExitCondition
			break
		}
	}

	r.writeFinal(fs, state)
	return reason, nil
}

func (r *typedLoopRunner[S]) writeFinal(fs *FlowState, state S) {
	Set(fs, r.cfg.stateKey, state)
	if r.cfg.finalToFS != nil {
		r.cfg.finalToFS(fs, state)
	}
}

// loopStep is the non-generic carrier stored on Step for loop execution.
type loopStep struct {
	runner loopRunner
}

// Loop returns a Step that re-executes a Steps body while While returns true,
// capped by MaxIterations. State is threaded between iterations as the typed
// value S; the framework persists S at the FlowState key declared via
// StateKey before/after each iteration so observability hooks and downstream
// readers see a consistent value.
//
// Determinism contract: Steps may invoke activities (no determinism
// constraint). The While predicate, BeforeIteration, AfterIteration,
// InitialState, FinalState, IterationHook, and MaxIterations all run in
// workflow code and must be deterministic.
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

// ThenLoop adds a Loop step to the builder under the given name.
func ThenLoop[S any](b *FlowBuilder, name string, opts ...LoopOption[S]) *FlowBuilder {
	s := Loop[S](opts...)
	s.name = name
	b.flow.steps = append(b.flow.steps, s)
	return b
}

// Steps sets the per-iteration Steps body. Required. Each iteration runs
// these Steps in order through the standard step executor, so observability
// hooks (BeforeStep / AfterStep / BeforeNode / AfterNode) fire normally.
func Steps[S any](steps ...Step) LoopOption[S] {
	return func(c *loopConfig[S]) { c.steps = steps }
}

// StateKey sets the FlowState key where the typed state S is persisted
// across iterations. Required. Reads/writes happen at the start and end of
// each iteration; downstream code can also read it via core.Get[S].
func StateKey[S any](key string) LoopOption[S] {
	return func(c *loopConfig[S]) { c.stateKey = key }
}

// While sets the continuation predicate. Evaluated after each iteration
// (after AfterIteration). Must be deterministic over S.
func While[S any](cond func(S) bool) LoopOption[S] {
	return func(c *loopConfig[S]) { c.while = cond }
}

// InitialState supplies the function that produces the seed S for the loop.
// Called once before the first iteration; the result is written to the
// FlowState key and threaded into iteration 1's BeforeIteration / Steps.
// Required.
func InitialState[S any](extract func(*FlowState) S) LoopOption[S] {
	return func(c *loopConfig[S]) { c.initialFromFS = extract }
}

// FinalState installs an optional callback that runs once after the last
// iteration to write the final S anywhere the caller wants (e.g. a different
// FlowState key for downstream Steps).
func FinalState[S any](inject func(*FlowState, S)) LoopOption[S] {
	return func(c *loopConfig[S]) { c.finalToFS = inject }
}

// BeforeIteration installs a callback that runs at the start of every
// iteration with mutating *FlowState access. Use it to drain signals,
// perform pre-flight checks, and update S in workflow code. Returning
// skipBody=true skips the Steps body for this iteration but still runs
// AfterIteration and the While predicate (useful for cancel signals).
//
// Runs in workflow code; must be deterministic.
func BeforeIteration[S any](fn func(s S, fs *FlowState) (S, bool)) LoopOption[S] {
	return func(c *loopConfig[S]) { c.beforeIteration = fn }
}

// AfterIteration installs a callback that runs at the end of every iteration
// with read-only FlowStateReader access. Use it to merge Step outputs into S.
// Returns the next S (written back to the StateKey).
//
// Runs in workflow code; must be deterministic.
func AfterIteration[S any](fn func(s S, fs FlowStateReader) S) LoopOption[S] {
	return func(c *loopConfig[S]) { c.afterIteration = fn }
}

// MaxIterations sets the hard ceiling on iteration count. Default 1000.
func MaxIterations[S any](n int) LoopOption[S] {
	return func(c *loopConfig[S]) { c.maxIterations = n }
}

// IterationHook installs a low-detail per-iteration callback for "iteration
// N started/completed" event emission. Distinct from FlowHooks (which observe
// Step/Node lifecycle) and from BeforeIteration/AfterIteration (which thread
// state). Runs in workflow code; must be deterministic.
func IterationHook[S any](h func(IterationEvent)) LoopOption[S] {
	return func(c *loopConfig[S]) { c.iterHook = h }
}

// ContinueAsNewAfter installs a continue-as-new policy. Default: never.
// Reserved; consumed when CAN emission lands in a future task.
func ContinueAsNewAfter[S any](p ContinueAsNewPolicy) LoopOption[S] {
	return func(c *loopConfig[S]) { c.canPolicy = p }
}

// IterationEvent is the payload delivered to an IterationHook.
type IterationEvent struct {
	Phase string // "started" | "completed"
	N     int    // 1-indexed iteration number
}

// ContinueAsNewPolicy controls when the loop emits ContinueAsNew.
type ContinueAsNewPolicy struct {
	AfterIterations   int
	AfterHistoryBytes int64
}

// LoopExitReason identifies why a Loop terminated.
type LoopExitReason string

const (
	LoopExitCondition     LoopExitReason = "condition"
	LoopExitMaxIterations LoopExitReason = "max_iterations"
	LoopExitCancelled     LoopExitReason = "cancelled"
	LoopExitContinueAsNew LoopExitReason = "continue_as_new"
)
