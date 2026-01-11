package core

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/workflow"
)

// Flow defines an executable workflow composed of steps.
type Flow struct {
	name        string
	trigger     Trigger
	steps       []Step
	stateConfig *StateConfig
}

// Step represents one execution unit within a flow.
// A step can contain one node (sequential), multiple nodes (parallel),
// or a conditional branch.
type Step struct {
	name        string
	nodes       []ExecutableNode
	parallel    bool
	conditional *ConditionalConfig
}

// FlowBuilder provides a fluent API for constructing flows.
type FlowBuilder struct {
	flow   *Flow
	errors []error
}

// NewFlow creates a new flow builder with the given name.
func NewFlow(name string) *FlowBuilder {
	return &FlowBuilder{
		flow: &Flow{
			name:  name,
			steps: make([]Step, 0),
		},
	}
}

// TriggeredBy sets the trigger that initiates this flow.
func (b *FlowBuilder) TriggeredBy(t Trigger) *FlowBuilder {
	b.flow.trigger = t
	return b
}

// Then adds a sequential step with a single node.
func (b *FlowBuilder) Then(node ExecutableNode) *FlowBuilder {
	if node == nil {
		b.errors = append(b.errors, fmt.Errorf("node cannot be nil"))
		return b
	}
	b.flow.steps = append(b.flow.steps, Step{
		name:     node.Name(),
		nodes:    []ExecutableNode{node},
		parallel: false,
	})
	return b
}

// ThenParallel adds a parallel step with multiple nodes executed concurrently.
func (b *FlowBuilder) ThenParallel(name string, nodes ...ExecutableNode) *FlowBuilder {
	if len(nodes) == 0 {
		b.errors = append(b.errors, fmt.Errorf("ThenParallel requires at least one node"))
		return b
	}
	for i, node := range nodes {
		if node == nil {
			b.errors = append(b.errors, fmt.Errorf("node at index %d cannot be nil", i))
			return b
		}
	}
	b.flow.steps = append(b.flow.steps, Step{
		name:     name,
		nodes:    nodes,
		parallel: true,
	})
	return b
}

// WithState overrides the default state backend (.resolute/).
// Use this to configure cloud storage backends (S3, GCS) for production.
func (b *FlowBuilder) WithState(cfg StateConfig) *FlowBuilder {
	b.flow.stateConfig = &cfg
	return b
}

// Build validates and returns the constructed flow.
func (b *FlowBuilder) Build() *Flow {
	if len(b.errors) > 0 {
		panic(fmt.Sprintf("flow build errors: %v", b.errors))
	}
	if len(b.flow.steps) == 0 {
		panic("flow must have at least one step")
	}
	if b.flow.trigger == nil {
		panic("flow must have a trigger")
	}
	return b.flow
}

// Name returns the flow's identifier.
func (f *Flow) Name() string {
	return f.name
}

// Trigger returns the flow's trigger configuration.
func (f *Flow) Trigger() Trigger {
	return f.trigger
}

// Steps returns the flow's execution steps.
func (f *Flow) Steps() []Step {
	return f.steps
}

// StateConfig returns the flow's state configuration, or nil for default.
func (f *Flow) StateConfig() *StateConfig {
	return f.stateConfig
}

// Execute runs the flow as a Temporal workflow.
func (f *Flow) Execute(ctx workflow.Context, input FlowInput) error {
	startTime := time.Now()

	err := f.executeInternal(ctx, input)

	status := "success"
	if err != nil {
		status = "error"
	}

	RecordFlowExecution(RecordFlowExecutionInput{
		FlowName: f.name,
		Status:   status,
		Duration: time.Since(startTime),
	})

	return err
}

func (f *Flow) executeInternal(ctx workflow.Context, input FlowInput) error {
	state := NewFlowState(input)

	if err := state.LoadPersisted(ctx, f.name, f.stateConfig); err != nil {
		return fmt.Errorf("load persisted state: %w", err)
	}

	var compensations []CompensationEntry

	for _, step := range f.steps {
		if step.conditional != nil {
			if err := executeConditional(ctx, step.conditional, state, f.name, &compensations); err != nil {
				return runCompensations(ctx, compensations, state, err)
			}
		} else if step.parallel {
			if err := executeParallel(ctx, step, state, f.name, &compensations); err != nil {
				return runCompensations(ctx, compensations, state, err)
			}
		} else {
			if err := executeSequential(ctx, step, state, f.name, &compensations); err != nil {
				return runCompensations(ctx, compensations, state, err)
			}
		}
	}

	if err := state.SavePersisted(ctx, f.name, f.stateConfig); err != nil {
		return fmt.Errorf("save persisted state: %w", err)
	}

	return nil
}

// executeSequential runs a single node and tracks compensation.
func executeSequential(ctx workflow.Context, step Step, state *FlowState, flowName string, compensations *[]CompensationEntry) error {
	node := step.nodes[0]

	if err := node.Execute(ctx, state); err != nil {
		return WrapFlowError(flowName, step.name, node.Name(), node.Input(), err)
	}

	// Track for compensation if node has one
	if node.HasCompensation() {
		*compensations = append(*compensations, CompensationEntry{
			node:  node,
			state: state.Snapshot(),
		})
	}

	return nil
}

// executeParallel runs multiple nodes concurrently and waits for all to complete.
func executeParallel(ctx workflow.Context, step Step, state *FlowState, flowName string, compensations *[]CompensationEntry) error {
	// Use Temporal's selector for parallel execution
	selector := workflow.NewSelector(ctx)
	results := make(chan error, len(step.nodes))

	for _, node := range step.nodes {
		n := node // capture for closure
		future := workflow.ExecuteActivity(ctx, func(actCtx workflow.Context) error {
			return n.Execute(actCtx, state)
		})
		selector.AddFuture(future, func(f workflow.Future) {
			results <- f.Get(ctx, nil)
		})

		// Track for compensation
		if n.HasCompensation() {
			*compensations = append(*compensations, CompensationEntry{
				node:  n,
				state: state.Snapshot(),
			})
		}
	}

	// Wait for all to complete
	for range step.nodes {
		selector.Select(ctx)
	}
	close(results)

	// Check for errors
	for err := range results {
		if err != nil {
			return WrapFlowError(flowName, step.name, "parallel", nil, err)
		}
	}

	return nil
}

// runCompensations executes compensation nodes in reverse order (Saga pattern).
func runCompensations(ctx workflow.Context, compensations []CompensationEntry, state *FlowState, originalErr error) error {
	// Execute compensations in reverse order
	for i := len(compensations) - 1; i >= 0; i-- {
		entry := compensations[i]
		if err := entry.node.Compensate(ctx, entry.state); err != nil {
			// Log compensation failure but continue with others
			workflow.GetLogger(ctx).Error("compensation failed",
				"node", entry.node.Name(),
				"error", err,
			)
		}
	}
	return originalErr
}

// FlowInput contains the initial input to a flow execution.
type FlowInput struct {
	Data map[string][]byte
}
