package core

import "go.temporal.io/sdk/workflow"

// ExecutableNode is the interface implemented by all nodes that can be executed
// within a flow. This allows type-erased storage of generic nodes in flow steps.
type ExecutableNode interface {
	// Name returns the node's identifier.
	Name() string

	// OutputKey returns the key used to store this node's output in FlowState.
	OutputKey() string

	// Execute runs the node within a Temporal workflow context.
	Execute(ctx workflow.Context, state *FlowState) error

	// Compensate runs the compensation logic if configured (Saga pattern).
	Compensate(ctx workflow.Context, state *FlowState) error

	// HasCompensation returns true if this node has compensation logic.
	HasCompensation() bool

	// Compensation returns the compensation node, if any.
	Compensation() ExecutableNode

	// Input returns the node's input value (used for testing).
	// Returns any because the actual type is generic.
	Input() any

	// RateLimiterID returns the ID of the rate limiter for this node, if any.
	// Returns empty string if no rate limiting is configured.
	RateLimiterID() string
}
