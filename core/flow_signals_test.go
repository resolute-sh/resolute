package core

import (
	"context"
	"testing"
)

func TestRegisterSignal(t *testing.T) {
	t.Parallel()

	t.Run("registers typed signal on flow", func(t *testing.T) {
		t.Parallel()
		// given a flow that registers two typed signals
		b := NewFlow("signal-test").
			TriggeredBy(Schedule("0 * * * *")).
			Then(NewNode("noop", func(ctx context.Context, _ struct{}) (struct{}, error) {
				return struct{}{}, nil
			}, struct{}{}))
		RegisterSignal[string](b, "steer")
		RegisterSignal[struct{}](b, "cancel")
		flow := b.Build()

		// then both signals are recorded with non-nil pumps
		if len(flow.Signals()) != 2 {
			t.Fatalf("Signals() = %d, want 2", len(flow.Signals()))
		}
		if flow.Signals()[0].Name != "steer" {
			t.Errorf("Signals()[0].Name = %q, want %q", flow.Signals()[0].Name, "steer")
		}
		if flow.Signals()[1].Name != "cancel" {
			t.Errorf("Signals()[1].Name = %q, want %q", flow.Signals()[1].Name, "cancel")
		}
		if flow.Signals()[0].pump == nil {
			t.Error("Signals()[0].pump = nil, want non-nil typed pump")
		}
		if flow.Signals()[1].pump == nil {
			t.Error("Signals()[1].pump = nil, want non-nil typed pump")
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

func TestTakeSignal_TypedRoundtrip(t *testing.T) {
	t.Parallel()

	type follow struct{ Text string }

	t.Run("typed inject + take returns typed value", func(t *testing.T) {
		t.Parallel()
		fs := NewFlowState(FlowInput{})

		// when a typed payload is injected and then taken
		InjectSignal[follow](fs, "follow", follow{Text: "hi"})
		got, ok := TakeSignal[follow](fs, "follow")

		// then the typed value comes back without any assertion at the call site
		if !ok {
			t.Fatal("TakeSignal returned ok=false, want true")
		}
		if got.Text != "hi" {
			t.Errorf("got.Text = %q, want %q", got.Text, "hi")
		}
	})

	t.Run("empty buffer returns zero + false", func(t *testing.T) {
		t.Parallel()
		fs := NewFlowState(FlowInput{})
		got, ok := TakeSignal[follow](fs, "follow")
		if ok {
			t.Errorf("TakeSignal on empty buffer ok = true, want false")
		}
		if got != (follow{}) {
			t.Errorf("TakeSignal on empty buffer value = %+v, want zero", got)
		}
	})

	t.Run("take is destructive", func(t *testing.T) {
		t.Parallel()
		fs := NewFlowState(FlowInput{})
		InjectSignal[follow](fs, "follow", follow{Text: "once"})
		_, _ = TakeSignal[follow](fs, "follow")
		_, ok := TakeSignal[follow](fs, "follow")
		if ok {
			t.Error("second TakeSignal after drain ok = true, want false")
		}
	})
}

func TestTakeAllSignals_DrainsBuffer(t *testing.T) {
	t.Parallel()

	type follow struct{ N int }
	fs := NewFlowState(FlowInput{})
	InjectSignal[follow](fs, "follow", follow{N: 1})
	InjectSignal[follow](fs, "follow", follow{N: 2})
	InjectSignal[follow](fs, "follow", follow{N: 3})

	all := TakeAllSignals[follow](fs, "follow")
	if len(all) != 3 {
		t.Fatalf("TakeAllSignals returned %d, want 3", len(all))
	}
	if all[0].N != 1 || all[1].N != 2 || all[2].N != 3 {
		t.Errorf("TakeAllSignals returned out of order: %+v", all)
	}
	if SignalCount(fs, "follow") != 0 {
		t.Errorf("SignalCount after TakeAll = %d, want 0", SignalCount(fs, "follow"))
	}
}

func TestPeekSignal_NonDestructive(t *testing.T) {
	t.Parallel()

	type follow struct{ Text string }
	fs := NewFlowState(FlowInput{})
	InjectSignal[follow](fs, "follow", follow{Text: "peek-me"})

	got1, ok1 := PeekSignal[follow](fs, "follow")
	got2, ok2 := PeekSignal[follow](fs, "follow")

	if !ok1 || !ok2 {
		t.Fatalf("PeekSignal twice: ok1=%v ok2=%v, want both true", ok1, ok2)
	}
	if got1.Text != "peek-me" || got2.Text != "peek-me" {
		t.Errorf("PeekSignal returned %+v / %+v, want both {peek-me}", got1, got2)
	}
	if SignalCount(fs, "follow") != 1 {
		t.Errorf("SignalCount after two peeks = %d, want 1", SignalCount(fs, "follow"))
	}
}

func TestTakeSignal_PanicsOnTypeMismatch(t *testing.T) {
	t.Parallel()

	type follow struct{ Text string }
	type other struct{ N int }

	fs := NewFlowState(FlowInput{})
	InjectSignal[follow](fs, "follow", follow{Text: "x"})

	defer func() {
		if r := recover(); r == nil {
			t.Error("TakeSignal with mismatched T did not panic — programmer error must surface loudly")
		}
	}()
	_, _ = TakeSignal[other](fs, "follow")
}
