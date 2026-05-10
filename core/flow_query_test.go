package core

import (
	"context"
	"testing"
)

func TestFlowBuilder_WithQueryState(t *testing.T) {
	t.Parallel()

	type counterState struct {
		Count int
		Label string
	}

	t.Run("registers a Build factory on the QueryDef", func(t *testing.T) {
		t.Parallel()

		// given a flow with WithQueryState wired
		b := NewFlow("query-state-test").
			TriggeredBy(Schedule("0 * * * *")).
			Then(NewNode("noop", func(ctx context.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}, struct{}{}))
		WithQueryState[counterState](b, "state", "loop.state")
		flow := b.Build()

		// then exactly one query is registered with Build set, Handler unset
		if got := len(flow.queries); got != 1 {
			t.Fatalf("queries = %d, want 1", got)
		}
		q := flow.queries[0]
		if q.Name != "state" {
			t.Errorf("queries[0].Name = %q, want %q", q.Name, "state")
		}
		if q.Build == nil {
			t.Errorf("queries[0].Build = nil, want non-nil factory")
		}
		if q.Handler != nil {
			t.Errorf("queries[0].Handler = %v, want nil", q.Handler)
		}
	})

	t.Run("Build returns a typed handler that reads from the StateKey", func(t *testing.T) {
		t.Parallel()

		// given a flow with a registered query and a populated FlowState
		b := NewFlow("query-state-read").
			TriggeredBy(Schedule("0 * * * *")).
			Then(NewNode("noop", func(ctx context.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}, struct{}{}))
		WithQueryState[counterState](b, "state", "loop.state")
		flow := b.Build()
		fs := NewFlowState(FlowInput{})
		Set(fs, "loop.state", counterState{Count: 7, Label: "ready"})

		// when the Build factory is invoked with the live FlowState
		raw := flow.queries[0].Build(fs)
		handler, ok := raw.(func() (counterState, error))
		if !ok {
			t.Fatalf("Build returned %T, want func() (counterState, error)", raw)
		}
		got, err := handler()

		// then the handler returns the latest typed value at that key
		if err != nil {
			t.Errorf("handler() error = %v, want nil", err)
		}
		if got.Count != 7 || got.Label != "ready" {
			t.Errorf("handler() = %+v, want {Count:7 Label:ready}", got)
		}
	})

	t.Run("handler returns zero value when StateKey is empty", func(t *testing.T) {
		t.Parallel()

		// given a flow whose StateKey has never been written
		b := NewFlow("query-state-empty").
			TriggeredBy(Schedule("0 * * * *")).
			Then(NewNode("noop", func(ctx context.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}, struct{}{}))
		WithQueryState[counterState](b, "state", "loop.state")
		flow := b.Build()
		fs := NewFlowState(FlowInput{})

		// when the handler runs against an empty FlowState
		handler := flow.queries[0].Build(fs).(func() (counterState, error))
		got, err := handler()

		// then it returns the zero value of S without error
		if err != nil {
			t.Errorf("handler() error = %v, want nil", err)
		}
		if got != (counterState{}) {
			t.Errorf("handler() = %+v, want zero value", got)
		}
	})

	t.Run("handler reflects later writes to the StateKey", func(t *testing.T) {
		t.Parallel()

		// given a flow whose handler is bound to a live FlowState
		b := NewFlow("query-state-live").
			TriggeredBy(Schedule("0 * * * *")).
			Then(NewNode("noop", func(ctx context.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}, struct{}{}))
		WithQueryState[counterState](b, "state", "loop.state")
		flow := b.Build()
		fs := NewFlowState(FlowInput{})
		handler := flow.queries[0].Build(fs).(func() (counterState, error))

		// when the StateKey is updated after the handler is built
		Set(fs, "loop.state", counterState{Count: 1, Label: "first"})
		first, _ := handler()
		Set(fs, "loop.state", counterState{Count: 2, Label: "second"})
		second, _ := handler()

		// then the handler observes the latest write each time it is called
		if first.Count != 1 || first.Label != "first" {
			t.Errorf("first = %+v, want {Count:1 Label:first}", first)
		}
		if second.Count != 2 || second.Label != "second" {
			t.Errorf("second = %+v, want {Count:2 Label:second}", second)
		}
	})
}
