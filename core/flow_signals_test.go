package core

import (
	"context"
	"testing"
)

func TestFlowBuilder_WithSignals(t *testing.T) {
	t.Parallel()

	t.Run("registers signals on flow", func(t *testing.T) {
		t.Parallel()
		flow := NewFlow("signal-test").
			TriggeredBy(Schedule("0 * * * *")).
			WithSignals(SignalDef{Name: "steer"}, SignalDef{Name: "cancel"}).
			Then(NewNode("noop", func(ctx context.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}, struct{}{})).
			Build()

		if len(flow.Signals()) != 2 {
			t.Errorf("Signals() = %d, want 2", len(flow.Signals()))
		}

		names := make([]string, len(flow.Signals()))
		for i, s := range flow.Signals() {
			names[i] = s.Name
		}

		if names[0] != "steer" {
			t.Errorf("Signals()[0].Name = %q, want %q", names[0], "steer")
		}
		if names[1] != "cancel" {
			t.Errorf("Signals()[1].Name = %q, want %q", names[1], "cancel")
		}
	})

	t.Run("empty signals by default", func(t *testing.T) {
		t.Parallel()
		flow := NewFlow("no-signals").
			TriggeredBy(Schedule("0 * * * *")).
			Then(NewNode("noop", func(ctx context.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}, struct{}{})).
			Build()

		if len(flow.Signals()) != 0 {
			t.Errorf("Signals() = %d, want 0", len(flow.Signals()))
		}
	})
}
