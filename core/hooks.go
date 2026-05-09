package core

import "time"

// FlowHooks defines observability callbacks invoked at flow, step, and node
// execution boundaries. All callbacks are optional — nil callbacks are safely
// skipped.
//
// Hooks run inside Temporal's deterministic workflow context. They must NOT
// perform I/O directly. They observe; they do not mutate. The FlowStateReader
// argument is read-only by design — mutations belong in Step execution and
// Iteration callbacks.
//
// For side effects (e.g. writing events to Postgres), enqueue activities
// from within the callback.
type FlowHooks struct {
	BeforeFlow func(HookContext, FlowStateReader)
	AfterFlow  func(HookContext, FlowStateReader)
	BeforeStep func(HookContext, FlowStateReader)
	AfterStep  func(HookContext, FlowStateReader)
	BeforeNode func(HookContext, FlowStateReader)
	AfterNode  func(HookContext, FlowStateReader)
}

// HookContext carries structured context about the current execution point.
type HookContext struct {
	FlowName string
	StepName string
	NodeName string
	Duration time.Duration // populated in After* callbacks only
	Error    error         // populated in After* callbacks only
}

// CostEntry represents a cost event derived from a node output (typically
// an LLM call). Emitted by user-supplied AfterNode hooks; the framework
// itself does not produce CostEntry values.
type CostEntry struct {
	NodeName  string
	Model     string
	Provider  string
	TokensIn  int
	TokensOut int
	CostUSD   float64
	Duration  time.Duration
	Metadata  map[string]string
}

func invokeBeforeFlow(hooks *FlowHooks, flowName string, fs FlowStateReader) {
	if hooks == nil || hooks.BeforeFlow == nil {
		return
	}
	hooks.BeforeFlow(HookContext{FlowName: flowName}, fs)
}

func invokeAfterFlow(hooks *FlowHooks, flowName string, duration time.Duration, err error, fs FlowStateReader) {
	if hooks == nil || hooks.AfterFlow == nil {
		return
	}
	hooks.AfterFlow(HookContext{FlowName: flowName, Duration: duration, Error: err}, fs)
}

func invokeBeforeStep(hooks *FlowHooks, flowName, stepName string, fs FlowStateReader) {
	if hooks == nil || hooks.BeforeStep == nil {
		return
	}
	hooks.BeforeStep(HookContext{FlowName: flowName, StepName: stepName}, fs)
}

func invokeAfterStep(hooks *FlowHooks, flowName, stepName string, duration time.Duration, err error, fs FlowStateReader) {
	if hooks == nil || hooks.AfterStep == nil {
		return
	}
	hooks.AfterStep(HookContext{FlowName: flowName, StepName: stepName, Duration: duration, Error: err}, fs)
}

func invokeBeforeNode(hooks *FlowHooks, flowName, stepName, nodeName string, fs FlowStateReader) {
	if hooks == nil || hooks.BeforeNode == nil {
		return
	}
	hooks.BeforeNode(HookContext{FlowName: flowName, StepName: stepName, NodeName: nodeName}, fs)
}

func invokeAfterNode(hooks *FlowHooks, flowName, stepName, nodeName string, duration time.Duration, err error, fs FlowStateReader) {
	if hooks == nil || hooks.AfterNode == nil {
		return
	}
	hooks.AfterNode(HookContext{FlowName: flowName, StepName: stepName, NodeName: nodeName, Duration: duration, Error: err}, fs)
}
