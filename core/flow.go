package core

import (
	"fmt"
	"reflect"
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

	for i, step := range f.steps {
		if step.parallel && hasWindowedNodes(step) {
			downstream := f.steps[i+1:]
			if err := executeWindowed(ctx, step, downstream, state, f.name, f.stateConfig); err != nil {
				return runCompensations(ctx, compensations, state, err)
			}
			if err := state.SavePersisted(ctx, f.name, f.stateConfig); err != nil {
				return fmt.Errorf("save persisted state: %w", err)
			}
			return nil
		}

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
	preSnapshot := state.Snapshot()

	errs := make([]error, len(step.nodes))
	doneCh := workflow.NewBufferedChannel(ctx, len(step.nodes))

	for i, node := range step.nodes {
		idx := i
		n := node
		workflow.Go(ctx, func(gCtx workflow.Context) {
			errs[idx] = n.Execute(gCtx, state)
			doneCh.Send(gCtx, idx)
		})
	}

	for range step.nodes {
		var idx int
		doneCh.Receive(ctx, &idx)
	}

	for i, err := range errs {
		if err == nil && step.nodes[i].HasCompensation() {
			*compensations = append(*compensations, CompensationEntry{
				node:  step.nodes[i],
				state: preSnapshot,
			})
		}
	}

	for i, err := range errs {
		if err != nil {
			return WrapFlowError(flowName, step.name, step.nodes[i].Name(), step.nodes[i].Input(), err)
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

// hasWindowedNodes returns true if any node in the step implements WindowedNode with Size > 0.
func hasWindowedNodes(step Step) bool {
	for _, node := range step.nodes {
		if wn, ok := node.(WindowedNode); ok && wn.WindowConfig().Size > 0 {
			return true
		}
	}
	return false
}

// executeWindowed runs windowed nodes in parallel, each looping through batches
// and executing the downstream pipeline per batch.
func executeWindowed(ctx workflow.Context, step Step, downstream []Step, state *FlowState, flowName string, stateConfig *StateConfig) error {
	errs := make([]error, len(step.nodes))
	doneCh := workflow.NewBufferedChannel(ctx, len(step.nodes))

	for i, node := range step.nodes {
		idx := i
		n := node
		workflow.Go(ctx, func(gCtx workflow.Context) {
			wn, ok := n.(WindowedNode)
			if !ok || wn.WindowConfig().Size == 0 {
				errs[idx] = n.Execute(gCtx, state)
			} else {
				errs[idx] = executeWindowedNode(gCtx, n, wn.WindowConfig(), downstream, state, flowName, stateConfig)
			}
			doneCh.Send(gCtx, idx)
		})
	}

	for range step.nodes {
		var idx int
		doneCh.Receive(ctx, &idx)
	}

	for i, err := range errs {
		if err != nil {
			return WrapFlowError(flowName, step.name, step.nodes[i].Name(), step.nodes[i].Input(), err)
		}
	}

	return nil
}

// executeWindowedNode runs a single windowed node in a loop, executing the
// downstream pipeline for each batch.
func executeWindowedNode(ctx workflow.Context, node ExecutableNode, windowCfg Window, downstream []Step, parentState *FlowState, flowName string, stateConfig *StateConfig) error {
	var windowCursor string

	for batchNum := 0; ; batchNum++ {
		batchState := parentState.NewBatchState()
		batchState.SetWindowMeta(windowCursor, windowCfg.Size)

		if err := node.Execute(ctx, batchState); err != nil {
			return fmt.Errorf("batch %d fetch: %w", batchNum, err)
		}

		result := batchState.GetResult(node.OutputKey())
		hasMore, nextCursor := extractWindowMeta(result)
		windowCursor = nextCursor

		batchState.SetResult("__window__", result)

		var compensations []CompensationEntry
		for _, step := range downstream {
			if step.conditional != nil {
				if err := executeConditional(ctx, step.conditional, batchState, flowName, &compensations); err != nil {
					return fmt.Errorf("batch %d downstream %s: %w", batchNum, step.name, err)
				}
			} else if step.parallel {
				if err := executeParallel(ctx, step, batchState, flowName, &compensations); err != nil {
					return fmt.Errorf("batch %d downstream %s: %w", batchNum, step.name, err)
				}
			} else {
				if err := executeSequential(ctx, step, batchState, flowName, &compensations); err != nil {
					return fmt.Errorf("batch %d downstream %s: %w", batchNum, step.name, err)
				}
			}
		}

		mergeCursors(parentState, batchState)

		if err := parentState.SavePersisted(ctx, flowName, stateConfig); err != nil {
			return fmt.Errorf("batch %d save state: %w", batchNum, err)
		}

		if !hasMore {
			break
		}
	}

	return nil
}

// extractWindowMeta reads HasMore and WindowCursor from a result struct via reflection.
func extractWindowMeta(result interface{}) (hasMore bool, cursor string) {
	if result == nil {
		return false, ""
	}
	val := reflect.ValueOf(result)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return false, ""
	}

	if f := val.FieldByName("HasMore"); f.IsValid() && f.Kind() == reflect.Bool {
		hasMore = f.Bool()
	}
	if f := val.FieldByName("WindowCursor"); f.IsValid() && f.Kind() == reflect.String {
		cursor = f.String()
	}
	return hasMore, cursor
}

// mergeCursors updates parent cursors with batch cursors, keeping the later position.
// RFC3339 string comparison preserves chronological order.
func mergeCursors(parent, batch *FlowState) {
	batch.mu.RLock()
	batchCursors := make(map[string]Cursor, len(batch.cursors))
	for k, v := range batch.cursors {
		batchCursors[k] = v
	}
	batch.mu.RUnlock()

	parent.mu.Lock()
	defer parent.mu.Unlock()
	for source, batchCursor := range batchCursors {
		parentCursor, exists := parent.cursors[source]
		if !exists || batchCursor.Position > parentCursor.Position {
			parent.cursors[source] = batchCursor
		}
	}
}

// FlowInput contains the initial input to a flow execution.
type FlowInput struct {
	Data map[string][]byte
}
