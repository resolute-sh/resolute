package core

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"
)

// GateConfig defines how a gate node pauses and resumes.
type GateConfig struct {
	SignalName string
	Timeout    time.Duration
}

// GateResult carries the outcome of a gate decision.
type GateResult struct {
	Approved   bool
	Decision   string
	DecidedBy  string
	DecidedAt  time.Time
	Reason     string
	Metadata   map[string]string
}

// GateTimeoutError indicates a gate was not resolved within the configured timeout.
type GateTimeoutError struct {
	GateName string
	Timeout  time.Duration
}

func (e *GateTimeoutError) Error() string {
	return fmt.Sprintf("gate %q timed out after %s", e.GateName, e.Timeout)
}

// GateNode pauses a flow until an external signal is received.
// It implements ExecutableNode so it can be placed in a Step.
type GateNode struct {
	name      string
	config    GateConfig
	outputKey string
}

// NewGateNode creates a gate node with the given name and configuration.
func NewGateNode(name string, config GateConfig) *GateNode {
	return &GateNode{
		name:   name,
		config: config,
	}
}

// As names the output of this gate for reference by downstream nodes.
func (g *GateNode) As(key string) *GateNode {
	g.outputKey = key
	return g
}

// Name returns the gate's identifier.
func (g *GateNode) Name() string {
	return g.name
}

// OutputKey returns the key used to store the gate result in FlowState.
func (g *GateNode) OutputKey() string {
	if g.outputKey != "" {
		return g.outputKey
	}
	return g.name
}

// Execute waits for a Temporal signal carrying a GateResult.
// If a timeout is configured, returns GateTimeoutError on expiry.
func (g *GateNode) Execute(ctx workflow.Context, state *FlowState) error {
	ch := workflow.GetSignalChannel(ctx, g.config.SignalName)
	var result GateResult

	if g.config.Timeout > 0 {
		timerCtx, cancelTimer := workflow.WithCancel(ctx)
		timerFuture := workflow.NewTimer(timerCtx, g.config.Timeout)

		selector := workflow.NewSelector(ctx)
		var resolved bool

		selector.AddReceive(ch, func(c workflow.ReceiveChannel, more bool) {
			c.Receive(ctx, &result)
			resolved = true
			cancelTimer()
		})

		selector.AddFuture(timerFuture, func(f workflow.Future) {
			_ = f.Get(ctx, nil)
		})

		selector.Select(ctx)

		if !resolved {
			return &GateTimeoutError{GateName: g.name, Timeout: g.config.Timeout}
		}
	} else {
		ch.Receive(ctx, &result)
	}

	result.DecidedAt = workflow.Now(ctx)
	state.SetResult(g.OutputKey(), result)
	return nil
}

// Compensate is a no-op for gates.
func (g *GateNode) Compensate(_ workflow.Context, _ *FlowState) error {
	return nil
}

// HasCompensation returns false — gates have no compensation.
func (g *GateNode) HasCompensation() bool {
	return false
}

// Compensation returns nil — gates have no compensation node.
func (g *GateNode) Compensation() ExecutableNode {
	return nil
}

// Input returns nil — gates have no input value.
func (g *GateNode) Input() interface{} {
	return nil
}

// RateLimiterID returns empty — gates don't use rate limiting.
func (g *GateNode) RateLimiterID() string {
	return ""
}
