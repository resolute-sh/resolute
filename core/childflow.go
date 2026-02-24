package core

import (
	"fmt"

	"go.temporal.io/sdk/workflow"
)

// ChildFlowNode spawns child workflows from a parent flow.
type ChildFlowNode struct {
	name      string
	config    ChildFlowConfig
	outputKey string
}

// ChildFlowConfig defines how child workflows are spawned from a parent flow.
type ChildFlowConfig struct {
	Flow        *Flow
	InputMapper func(*FlowState) []FlowInput
	Sequential  bool
}

// ChildFlowResults holds the outcomes of all spawned child workflows.
type ChildFlowResults struct {
	States []*FlowState
	Errors []error
	Count  int
}

// NewChildFlowNode creates a child flow node with the given name and configuration.
func NewChildFlowNode(name string, config ChildFlowConfig) *ChildFlowNode {
	return &ChildFlowNode{
		name:   name,
		config: config,
	}
}

// As names the output of this child flow node for reference by downstream nodes.
func (c *ChildFlowNode) As(key string) *ChildFlowNode {
	c.outputKey = key
	return c
}

// Name returns the child flow node's identifier.
func (c *ChildFlowNode) Name() string {
	return c.name
}

// OutputKey returns the key used to store child flow results in FlowState.
func (c *ChildFlowNode) OutputKey() string {
	if c.outputKey != "" {
		return c.outputKey
	}
	return c.name
}

// Execute spawns child workflows using workflow.ExecuteChildWorkflow.
// Parallel by default; sequential if config.Sequential is true.
func (c *ChildFlowNode) Execute(ctx workflow.Context, state *FlowState) error {
	if c.config.InputMapper == nil {
		return fmt.Errorf("child flow %s: InputMapper is required", c.name)
	}
	if c.config.Flow == nil {
		return fmt.Errorf("child flow %s: Flow is required", c.name)
	}

	inputs := c.config.InputMapper(state)
	if len(inputs) == 0 {
		state.SetResult(c.OutputKey(), ChildFlowResults{Count: 0})
		return nil
	}

	if c.config.Sequential {
		return c.executeSequential(ctx, state, inputs)
	}
	return c.executeParallel(ctx, state, inputs)
}

func (c *ChildFlowNode) executeParallel(ctx workflow.Context, state *FlowState, inputs []FlowInput) error {
	futures := make([]workflow.ChildWorkflowFuture, len(inputs))

	for i, input := range inputs {
		childID := fmt.Sprintf("%s-child-%d", c.name, i)
		childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
			WorkflowID: childID,
		})
		futures[i] = workflow.ExecuteChildWorkflow(childCtx, c.config.Flow.Execute, input)
	}

	results := ChildFlowResults{
		Count:  len(inputs),
		Errors: make([]error, len(inputs)),
	}

	var firstErr error
	for i, future := range futures {
		err := future.Get(ctx, nil)
		results.Errors[i] = err
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("child flow %s[%d]: %w", c.name, i, err)
		}
	}

	state.SetResult(c.OutputKey(), results)
	return firstErr
}

func (c *ChildFlowNode) executeSequential(ctx workflow.Context, state *FlowState, inputs []FlowInput) error {
	results := ChildFlowResults{
		Count:  len(inputs),
		Errors: make([]error, len(inputs)),
	}

	for i, input := range inputs {
		childID := fmt.Sprintf("%s-child-%d", c.name, i)
		childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
			WorkflowID: childID,
		})
		future := workflow.ExecuteChildWorkflow(childCtx, c.config.Flow.Execute, input)
		err := future.Get(ctx, nil)
		results.Errors[i] = err

		if err != nil {
			state.SetResult(c.OutputKey(), results)
			return fmt.Errorf("child flow %s[%d]: %w", c.name, i, err)
		}
	}

	state.SetResult(c.OutputKey(), results)
	return nil
}

// Compensate is a no-op for child flow nodes.
func (c *ChildFlowNode) Compensate(_ workflow.Context, _ *FlowState) error {
	return nil
}

// HasCompensation returns false — child flow nodes have no compensation.
func (c *ChildFlowNode) HasCompensation() bool {
	return false
}

// Compensation returns nil.
func (c *ChildFlowNode) Compensation() ExecutableNode {
	return nil
}

// Input returns nil — child flow nodes derive input from InputMapper.
func (c *ChildFlowNode) Input() interface{} {
	return nil
}

// RateLimiterID returns empty — child flows don't use rate limiting.
func (c *ChildFlowNode) RateLimiterID() string {
	return ""
}
