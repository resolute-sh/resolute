package core

import (
	"fmt"

	"go.temporal.io/sdk/workflow"
)

// Predicate evaluates flow state to determine branching.
type Predicate func(*FlowState) bool

// ConditionalConfig defines the structure of a conditional step.
type ConditionalConfig struct {
	predicate Predicate
	thenSteps []Step
	elseSteps []Step
}

// ConditionalBuilder provides a fluent API for building conditional branches.
type ConditionalBuilder struct {
	parent    *FlowBuilder
	config    *ConditionalConfig
	inElse    bool
	errors    []error
}

// When starts a conditional branch based on a predicate.
// The predicate is evaluated at runtime against the current FlowState.
//
// Example:
//
//	flow := NewFlow("order-flow").
//	    TriggeredBy(Manual("test")).
//	    Then(fetchOrder).
//	    When(func(s *FlowState) bool {
//	        return Get[Order](s, "order").Total > 1000
//	    }).
//	    Then(requireApproval).
//	    Otherwise(autoApprove).
//	    Then(fulfillOrder).
//	    Build()
func (b *FlowBuilder) When(pred Predicate) *ConditionalBuilder {
	if pred == nil {
		b.errors = append(b.errors, fmt.Errorf("predicate cannot be nil"))
		return &ConditionalBuilder{parent: b, config: &ConditionalConfig{}}
	}
	return &ConditionalBuilder{
		parent: b,
		config: &ConditionalConfig{
			predicate: pred,
			thenSteps: make([]Step, 0),
			elseSteps: make([]Step, 0),
		},
	}
}

// Then adds a sequential step to the "then" branch (when predicate is true).
func (cb *ConditionalBuilder) Then(node ExecutableNode) *ConditionalBuilder {
	if node == nil {
		cb.errors = append(cb.errors, fmt.Errorf("node cannot be nil"))
		return cb
	}
	step := Step{
		name:     node.Name(),
		nodes:    []ExecutableNode{node},
		parallel: false,
	}
	if cb.inElse {
		cb.config.elseSteps = append(cb.config.elseSteps, step)
	} else {
		cb.config.thenSteps = append(cb.config.thenSteps, step)
	}
	return cb
}

// ThenParallel adds a parallel step to the current branch.
func (cb *ConditionalBuilder) ThenParallel(name string, nodes ...ExecutableNode) *ConditionalBuilder {
	if len(nodes) == 0 {
		cb.errors = append(cb.errors, fmt.Errorf("ThenParallel requires at least one node"))
		return cb
	}
	for i, node := range nodes {
		if node == nil {
			cb.errors = append(cb.errors, fmt.Errorf("node at index %d cannot be nil", i))
			return cb
		}
	}
	step := Step{
		name:     name,
		nodes:    nodes,
		parallel: true,
	}
	if cb.inElse {
		cb.config.elseSteps = append(cb.config.elseSteps, step)
	} else {
		cb.config.thenSteps = append(cb.config.thenSteps, step)
	}
	return cb
}

// Else switches to building the "else" branch (when predicate is false).
// After calling Else, subsequent Then/ThenParallel calls add to the else branch.
func (cb *ConditionalBuilder) Else() *ConditionalBuilder {
	if cb.inElse {
		cb.errors = append(cb.errors, fmt.Errorf("Else already called"))
		return cb
	}
	cb.inElse = true
	return cb
}

// Otherwise adds a single node to the "else" branch and returns to the main flow builder.
// This is a convenience method for simple conditionals with a single else action.
func (cb *ConditionalBuilder) Otherwise(node ExecutableNode) *FlowBuilder {
	if node == nil {
		cb.errors = append(cb.errors, fmt.Errorf("node cannot be nil"))
		return cb.EndWhen()
	}
	cb.config.elseSteps = append(cb.config.elseSteps, Step{
		name:     node.Name(),
		nodes:    []ExecutableNode{node},
		parallel: false,
	})
	return cb.EndWhen()
}

// OtherwiseParallel adds parallel nodes to the "else" branch and returns to the main flow builder.
func (cb *ConditionalBuilder) OtherwiseParallel(name string, nodes ...ExecutableNode) *FlowBuilder {
	if len(nodes) == 0 {
		cb.errors = append(cb.errors, fmt.Errorf("OtherwiseParallel requires at least one node"))
		return cb.EndWhen()
	}
	for i, node := range nodes {
		if node == nil {
			cb.errors = append(cb.errors, fmt.Errorf("node at index %d cannot be nil", i))
			return cb.EndWhen()
		}
	}
	cb.config.elseSteps = append(cb.config.elseSteps, Step{
		name:     name,
		nodes:    nodes,
		parallel: true,
	})
	return cb.EndWhen()
}

// EndWhen completes the conditional block and returns to the main flow builder.
// Use this when you don't need an else branch.
func (cb *ConditionalBuilder) EndWhen() *FlowBuilder {
	cb.parent.errors = append(cb.parent.errors, cb.errors...)
	if len(cb.config.thenSteps) == 0 {
		cb.parent.errors = append(cb.parent.errors, fmt.Errorf("conditional must have at least one step in then branch"))
		return cb.parent
	}
	cb.parent.flow.steps = append(cb.parent.flow.steps, Step{
		name:        "conditional",
		conditional: cb.config,
	})
	return cb.parent
}

// executeConditional evaluates the predicate and executes the appropriate branch.
func executeConditional(ctx workflow.Context, config *ConditionalConfig, state *FlowState, flowName string, compensations *[]CompensationEntry, hooks *FlowHooks) error {
	var stepsToExecute []Step
	if config.predicate(state) {
		stepsToExecute = config.thenSteps
	} else {
		stepsToExecute = config.elseSteps
	}

	for _, step := range stepsToExecute {
		if step.conditional != nil {
			if err := executeConditional(ctx, step.conditional, state, flowName, compensations, hooks); err != nil {
				return err
			}
		} else if step.parallel {
			if err := executeParallel(ctx, step, state, flowName, compensations, hooks); err != nil {
				return err
			}
		} else {
			if err := executeSequential(ctx, step, state, flowName, compensations, hooks); err != nil {
				return err
			}
		}
	}

	return nil
}
