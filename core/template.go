package core

import "fmt"

// FlowTemplate constructs flows dynamically from runtime configuration.
// Unlike FlowBuilder's compile-time fluent chain, FlowTemplate accepts
// step definitions that can vary per execution (e.g. different iteration
// phases running different subsets of steps).
type FlowTemplate struct {
	name    string
	trigger Trigger
	hooks   *FlowHooks
	state   *StateConfig
	steps   []templateStep
	errors  []error
}

type templateStep struct {
	name     string
	kind     templateStepKind
	node     ExecutableNode
	nodes    []ExecutableNode
	gate     *GateNode
	children *ChildFlowNode
	cond     *templateConditional
}

type templateStepKind int

const (
	templateStepSequential templateStepKind = iota
	templateStepParallel
	templateStepGate
	templateStepChildren
	templateStepConditional
)

type templateConditional struct {
	predicate Predicate
	thenSteps []templateStep
	elseSteps []templateStep
}

// NewFlowTemplate creates a new template for dynamic flow construction.
func NewFlowTemplate(name string) *FlowTemplate {
	return &FlowTemplate{
		name: name,
	}
}

// TriggeredBy sets the trigger for the flow.
func (ft *FlowTemplate) TriggeredBy(t Trigger) *FlowTemplate {
	ft.trigger = t
	return ft
}

// WithHooks attaches lifecycle callbacks.
func (ft *FlowTemplate) WithHooks(hooks *FlowHooks) *FlowTemplate {
	ft.hooks = hooks
	return ft
}

// WithState sets the state persistence configuration.
func (ft *FlowTemplate) WithState(cfg StateConfig) *FlowTemplate {
	ft.state = &cfg
	return ft
}

// AddStep adds a sequential step with a single node.
func (ft *FlowTemplate) AddStep(node ExecutableNode) *FlowTemplate {
	if node == nil {
		ft.errors = append(ft.errors, fmt.Errorf("AddStep: node cannot be nil"))
		return ft
	}
	ft.steps = append(ft.steps, templateStep{
		name: node.Name(),
		kind: templateStepSequential,
		node: node,
	})
	return ft
}

// AddParallel adds a parallel step with multiple nodes.
func (ft *FlowTemplate) AddParallel(name string, nodes ...ExecutableNode) *FlowTemplate {
	if len(nodes) == 0 {
		ft.errors = append(ft.errors, fmt.Errorf("AddParallel %q: requires at least one node", name))
		return ft
	}
	for i, node := range nodes {
		if node == nil {
			ft.errors = append(ft.errors, fmt.Errorf("AddParallel %q: node at index %d cannot be nil", name, i))
			return ft
		}
	}
	ft.steps = append(ft.steps, templateStep{
		name:  name,
		kind:  templateStepParallel,
		nodes: nodes,
	})
	return ft
}

// AddGate adds a gate step that pauses until a signal is received.
func (ft *FlowTemplate) AddGate(name string, config GateConfig) *FlowTemplate {
	if config.SignalName == "" {
		ft.errors = append(ft.errors, fmt.Errorf("AddGate %q: SignalName is required", name))
		return ft
	}
	ft.steps = append(ft.steps, templateStep{
		name: name,
		kind: templateStepGate,
		gate: NewGateNode(name, config),
	})
	return ft
}

// AddChildren adds a child workflow spawning step.
func (ft *FlowTemplate) AddChildren(name string, config ChildFlowConfig) *FlowTemplate {
	if config.Flow == nil {
		ft.errors = append(ft.errors, fmt.Errorf("AddChildren %q: Flow is required", name))
		return ft
	}
	if config.InputMapper == nil {
		ft.errors = append(ft.errors, fmt.Errorf("AddChildren %q: InputMapper is required", name))
		return ft
	}
	ft.steps = append(ft.steps, templateStep{
		name:     name,
		kind:     templateStepChildren,
		children: NewChildFlowNode(name, config),
	})
	return ft
}

// AddConditional adds a conditional branch evaluated at runtime.
func (ft *FlowTemplate) AddConditional(pred Predicate, thenSteps []ExecutableNode, elseSteps []ExecutableNode) *FlowTemplate {
	if pred == nil {
		ft.errors = append(ft.errors, fmt.Errorf("AddConditional: predicate cannot be nil"))
		return ft
	}
	if len(thenSteps) == 0 {
		ft.errors = append(ft.errors, fmt.Errorf("AddConditional: requires at least one then step"))
		return ft
	}

	cond := &templateConditional{
		predicate: pred,
	}

	for _, node := range thenSteps {
		cond.thenSteps = append(cond.thenSteps, templateStep{
			name: node.Name(),
			kind: templateStepSequential,
			node: node,
		})
	}
	for _, node := range elseSteps {
		cond.elseSteps = append(cond.elseSteps, templateStep{
			name: node.Name(),
			kind: templateStepSequential,
			node: node,
		})
	}

	ft.steps = append(ft.steps, templateStep{
		name: "conditional",
		kind: templateStepConditional,
		cond: cond,
	})
	return ft
}

// Build validates and returns a Flow from the template.
func (ft *FlowTemplate) Build() *Flow {
	if len(ft.errors) > 0 {
		panic(fmt.Sprintf("flow template build errors: %v", ft.errors))
	}
	if len(ft.steps) == 0 {
		panic("flow template must have at least one step")
	}
	if ft.trigger == nil {
		panic("flow template must have a trigger")
	}

	flow := &Flow{
		name:        ft.name,
		trigger:     ft.trigger,
		hooks:       ft.hooks,
		stateConfig: ft.state,
		steps:       make([]Step, 0, len(ft.steps)),
	}

	for _, ts := range ft.steps {
		flow.steps = append(flow.steps, ft.convertStep(ts))
	}

	return flow
}

func (ft *FlowTemplate) convertStep(ts templateStep) Step {
	switch ts.kind {
	case templateStepSequential:
		return Step{
			name:  ts.name,
			nodes: []ExecutableNode{ts.node},
		}
	case templateStepParallel:
		return Step{
			name:     ts.name,
			nodes:    ts.nodes,
			parallel: true,
		}
	case templateStepGate:
		return Step{
			name: ts.name,
			gate: ts.gate,
		}
	case templateStepChildren:
		return Step{
			name:     ts.name,
			children: ts.children,
		}
	case templateStepConditional:
		config := &ConditionalConfig{
			predicate: ts.cond.predicate,
			thenSteps: make([]Step, 0, len(ts.cond.thenSteps)),
			elseSteps: make([]Step, 0, len(ts.cond.elseSteps)),
		}
		for _, s := range ts.cond.thenSteps {
			config.thenSteps = append(config.thenSteps, ft.convertStep(s))
		}
		for _, s := range ts.cond.elseSteps {
			config.elseSteps = append(config.elseSteps, ft.convertStep(s))
		}
		return Step{
			name:        "conditional",
			conditional: config,
		}
	default:
		panic(fmt.Sprintf("unknown template step kind: %d", ts.kind))
	}
}
