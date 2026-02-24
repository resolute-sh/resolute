package core

import "time"

// FlowHooks defines callbacks invoked at flow, step, and node execution boundaries.
// All callbacks are optional — nil callbacks are safely skipped.
//
// Hooks run inside Temporal's deterministic workflow context. They must NOT
// perform I/O directly. For side effects (e.g. writing events to Postgres),
// enqueue activities from within the callback.
type FlowHooks struct {
	BeforeFlow func(HookContext)
	AfterFlow  func(HookContext)
	BeforeStep func(HookContext)
	AfterStep  func(HookContext)
	BeforeNode func(HookContext)
	AfterNode  func(HookContext)
	OnCost     func(CostEntry)
}

// HookContext carries structured context about the current execution point.
type HookContext struct {
	FlowName string
	StepName string
	NodeName string
	Duration time.Duration // populated in After* callbacks only
	Error    error         // populated in After* callbacks only
}

// CostEntry represents a cost event emitted by a node (typically an LLM call).
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

func invokeBeforeFlow(hooks *FlowHooks, flowName string) {
	if hooks == nil || hooks.BeforeFlow == nil {
		return
	}
	hooks.BeforeFlow(HookContext{FlowName: flowName})
}

func invokeAfterFlow(hooks *FlowHooks, flowName string, duration time.Duration, err error) {
	if hooks == nil || hooks.AfterFlow == nil {
		return
	}
	hooks.AfterFlow(HookContext{FlowName: flowName, Duration: duration, Error: err})
}

func invokeBeforeStep(hooks *FlowHooks, flowName, stepName string) {
	if hooks == nil || hooks.BeforeStep == nil {
		return
	}
	hooks.BeforeStep(HookContext{FlowName: flowName, StepName: stepName})
}

func invokeAfterStep(hooks *FlowHooks, flowName, stepName string, duration time.Duration, err error) {
	if hooks == nil || hooks.AfterStep == nil {
		return
	}
	hooks.AfterStep(HookContext{FlowName: flowName, StepName: stepName, Duration: duration, Error: err})
}

func invokeBeforeNode(hooks *FlowHooks, flowName, stepName, nodeName string) {
	if hooks == nil || hooks.BeforeNode == nil {
		return
	}
	hooks.BeforeNode(HookContext{FlowName: flowName, StepName: stepName, NodeName: nodeName})
}

func invokeAfterNode(hooks *FlowHooks, flowName, stepName, nodeName string, duration time.Duration, err error) {
	if hooks == nil || hooks.AfterNode == nil {
		return
	}
	hooks.AfterNode(HookContext{FlowName: flowName, StepName: stepName, NodeName: nodeName, Duration: duration, Error: err})
}

func invokeCost(hooks *FlowHooks, entry CostEntry) {
	if hooks == nil || hooks.OnCost == nil {
		return
	}
	hooks.OnCost(entry)
}
