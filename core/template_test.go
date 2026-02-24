package core

import (
	"context"
	"sync"
	"testing"
)

type tmplInput struct {
	Value string
}

type tmplOutput struct {
	Result string
}

func TestFlowTemplate_BasicConstruction(t *testing.T) {
	t.Parallel()

	// given
	node1 := NewNode("step1", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{})

	node2 := NewNode("step2", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{})

	flow := NewFlowTemplate("dynamic-flow").
		TriggeredBy(Manual("test")).
		AddStep(node1).
		AddStep(node2).
		Build()

	tester := NewFlowTester().
		MockValue("step1", tmplOutput{Result: "1"}).
		MockValue("step2", tmplOutput{Result: "2"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "step1")
	tester.AssertCalled(t, "step2")

	r1 := Get[tmplOutput](state, "step1")
	if r1.Result != "1" {
		t.Errorf("step1 result: got %q, want %q", r1.Result, "1")
	}
}

func TestFlowTemplate_DynamicStepList(t *testing.T) {
	t.Parallel()

	// given — simulate dynamic phase scoping
	type phase struct {
		name string
		node ExecutableNode
	}

	allPhases := []phase{
		{"intake", NewNode("intake", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})},
		{"refine", NewNode("refine", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})},
		{"implement", NewNode("implement", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})},
		{"verify", NewNode("verify", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})},
	}

	activePhases := []string{"implement", "verify"}

	tmpl := NewFlowTemplate("iteration-flow").
		TriggeredBy(Manual("test"))

	for _, p := range allPhases {
		for _, active := range activePhases {
			if p.name == active {
				tmpl.AddStep(p.node)
				break
			}
		}
	}

	flow := tmpl.Build()

	tester := NewFlowTester().
		MockValue("intake", tmplOutput{Result: "intake"}).
		MockValue("refine", tmplOutput{Result: "refine"}).
		MockValue("implement", tmplOutput{Result: "impl"}).
		MockValue("verify", tmplOutput{Result: "verified"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertNotCalled(t, "intake")
	tester.AssertNotCalled(t, "refine")
	tester.AssertCalled(t, "implement")
	tester.AssertCalled(t, "verify")

	r := Get[tmplOutput](state, "verify")
	if r.Result != "verified" {
		t.Errorf("verify result: got %q, want %q", r.Result, "verified")
	}
}

func TestFlowTemplate_WithParallel(t *testing.T) {
	t.Parallel()

	// given
	p1 := NewNode("p1", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{}).As("p1-out")

	p2 := NewNode("p2", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{}).As("p2-out")

	flow := NewFlowTemplate("parallel-tmpl").
		TriggeredBy(Manual("test")).
		AddParallel("batch", p1, p2).
		Build()

	tester := NewFlowTester().
		MockValue("p1", tmplOutput{Result: "a"}).
		MockValue("p2", tmplOutput{Result: "b"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r1 := Get[tmplOutput](state, "p1-out")
	if r1.Result != "a" {
		t.Errorf("p1 result: got %q, want %q", r1.Result, "a")
	}

	r2 := Get[tmplOutput](state, "p2-out")
	if r2.Result != "b" {
		t.Errorf("p2 result: got %q, want %q", r2.Result, "b")
	}
}

func TestFlowTemplate_WithGate(t *testing.T) {
	t.Parallel()

	// given
	node := NewNode("work", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{})

	flow := NewFlowTemplate("gate-tmpl").
		TriggeredBy(Manual("test")).
		AddStep(node).
		AddGate("approval", GateConfig{SignalName: "approval_signal"}).
		Build()

	tester := NewFlowTester().
		MockValue("work", tmplOutput{Result: "done"}).
		MockGate("approval", GateResult{Approved: true, Decision: "approved"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "work")
	tester.AssertCalled(t, "approval")

	gate := Get[GateResult](state, "approval")
	if !gate.Approved {
		t.Error("expected gate to be approved")
	}
}

func TestFlowTemplate_WithChildren(t *testing.T) {
	t.Parallel()

	// given
	childFlow := NewFlow("child").
		TriggeredBy(Manual("test")).
		Then(NewNode("child-step", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})).
		Build()

	flow := NewFlowTemplate("children-tmpl").
		TriggeredBy(Manual("test")).
		AddChildren("spawn", ChildFlowConfig{
			Flow: childFlow,
			InputMapper: func(s *FlowState) []FlowInput {
				return []FlowInput{{}, {}}
			},
		}).
		Build()

	tester := NewFlowTester().
		MockChildFlow("spawn", func(s *FlowState) (*FlowState, error) {
			return nil, nil
		})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "spawn")
}

func TestFlowTemplate_WithConditional(t *testing.T) {
	t.Parallel()

	// given
	setupNode := NewNode("setup", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{}).As("setup-result")

	thenNode := NewNode("then-action", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{})

	elseNode := NewNode("else-action", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{})

	flow := NewFlowTemplate("cond-tmpl").
		TriggeredBy(Manual("test")).
		AddStep(setupNode).
		AddConditional(
			func(s *FlowState) bool {
				return Get[tmplOutput](s, "setup-result").Result == "go"
			},
			[]ExecutableNode{thenNode},
			[]ExecutableNode{elseNode},
		).
		Build()

	tester := NewFlowTester().
		MockValue("setup", tmplOutput{Result: "go"}).
		MockValue("then-action", tmplOutput{Result: "then"}).
		MockValue("else-action", tmplOutput{Result: "else"})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tester.AssertCalled(t, "setup")
	tester.AssertCalled(t, "then-action")
	tester.AssertNotCalled(t, "else-action")
}

func TestFlowTemplate_WithHooks(t *testing.T) {
	t.Parallel()

	// given
	var mu sync.Mutex
	var trace []string
	record := func(event string) {
		mu.Lock()
		trace = append(trace, event)
		mu.Unlock()
	}

	hooks := &FlowHooks{
		BeforeFlow: func(hc HookContext) { record("BeforeFlow") },
		AfterFlow:  func(hc HookContext) { record("AfterFlow") },
		BeforeNode: func(hc HookContext) { record("BeforeNode:" + hc.NodeName) },
	}

	node := NewNode("work", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{})

	flow := NewFlowTemplate("hooked-tmpl").
		TriggeredBy(Manual("test")).
		WithHooks(hooks).
		AddStep(node).
		Build()

	tester := NewFlowTester().
		MockValue("work", tmplOutput{Result: "done"})

	// when
	_, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(trace) < 3 {
		t.Fatalf("expected at least 3 trace entries, got %d: %v", len(trace), trace)
	}

	if trace[0] != "BeforeFlow" {
		t.Errorf("trace[0]: got %q, want %q", trace[0], "BeforeFlow")
	}
	if trace[len(trace)-1] != "AfterFlow" {
		t.Errorf("trace[last]: got %q, want %q", trace[len(trace)-1], "AfterFlow")
	}
}

func TestFlowTemplate_FullPipeline(t *testing.T) {
	t.Parallel()

	// given — simulate BSaaE iteration with gate → work → children → gate → finalize
	childFlow := NewFlow("story-flow").
		TriggeredBy(Manual("test")).
		Then(NewNode("story-work", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})).
		Build()

	flow := NewFlowTemplate("full-iteration").
		TriggeredBy(Manual("test")).
		AddStep(NewNode("classify", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})).
		AddStep(NewNode("refine", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})).
		AddGate("requirements-review", GateConfig{SignalName: "req_review"}).
		AddStep(NewNode("plan", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})).
		AddGate("design-review", GateConfig{SignalName: "design_review"}).
		AddChildren("implement-stories", ChildFlowConfig{
			Flow: childFlow,
			InputMapper: func(s *FlowState) []FlowInput {
				return []FlowInput{{}, {}, {}}
			},
		}).
		AddStep(NewNode("verify", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})).
		AddGate("release-review", GateConfig{SignalName: "release_review"}).
		AddStep(NewNode("deploy", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})).
		Build()

	tester := NewFlowTester().
		MockValue("classify", tmplOutput{Result: "classified"}).
		MockValue("refine", tmplOutput{Result: "refined"}).
		MockGate("requirements-review", GateResult{Approved: true}).
		MockValue("plan", tmplOutput{Result: "planned"}).
		MockGate("design-review", GateResult{Approved: true}).
		MockChildFlow("implement-stories", func(s *FlowState) (*FlowState, error) {
			return nil, nil
		}).
		MockValue("verify", tmplOutput{Result: "verified"}).
		MockGate("release-review", GateResult{Approved: true}).
		MockValue("deploy", tmplOutput{Result: "deployed"})

	// when
	state, err := tester.Run(flow, FlowInput{})

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"classify", "refine", "requirements-review", "plan", "design-review", "implement-stories", "verify", "release-review", "deploy"} {
		tester.AssertCalled(t, name)
	}

	deploy := Get[tmplOutput](state, "deploy")
	if deploy.Result != "deployed" {
		t.Errorf("deploy result: got %q, want %q", deploy.Result, "deployed")
	}
}

func TestFlowTemplate_EmptyBuildPanics(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
	}()

	NewFlowTemplate("empty").
		TriggeredBy(Manual("test")).
		Build()
}

func TestFlowTemplate_NoTriggerPanics(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
	}()

	NewFlowTemplate("no-trigger").
		AddStep(NewNode("step", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
			return tmplOutput{}, nil
		}, tmplInput{})).
		Build()
}

func TestFlowTemplate_ValidationErrors(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil node, got none")
		}
	}()

	NewFlowTemplate("bad-tmpl").
		TriggeredBy(Manual("test")).
		AddStep(nil).
		Build()
}

func TestFlowTemplate_ProducesEquivalentFlow(t *testing.T) {
	t.Parallel()

	// given — build same flow with builder and template, ensure equivalent behavior
	node1 := NewNode("step1", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{})

	node2 := NewNode("step2", func(ctx context.Context, in tmplInput) (tmplOutput, error) {
		return tmplOutput{}, nil
	}, tmplInput{})

	builderFlow := NewFlow("equiv").
		TriggeredBy(Manual("test")).
		Then(node1).
		ThenGate("gate", GateConfig{SignalName: "sig"}).
		Then(node2).
		Build()

	templateFlow := NewFlowTemplate("equiv").
		TriggeredBy(Manual("test")).
		AddStep(node1).
		AddGate("gate", GateConfig{SignalName: "sig"}).
		AddStep(node2).
		Build()

	tester := NewFlowTester().
		MockValue("step1", tmplOutput{Result: "a"}).
		MockGate("gate", GateResult{Approved: true}).
		MockValue("step2", tmplOutput{Result: "b"})

	// when
	state1, err1 := tester.Run(builderFlow, FlowInput{})
	tester.Reset()
	state2, err2 := tester.Run(templateFlow, FlowInput{})

	// then
	if err1 != nil {
		t.Fatalf("builder flow error: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("template flow error: %v", err2)
	}

	r1 := Get[tmplOutput](state1, "step2")
	r2 := Get[tmplOutput](state2, "step2")
	if r1.Result != r2.Result {
		t.Errorf("builder result %q != template result %q", r1.Result, r2.Result)
	}
}
