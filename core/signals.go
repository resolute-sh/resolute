package core

import "go.temporal.io/sdk/workflow"

// RegisterSignal[T] declares that this flow accepts a signal named `name`
// whose payload deserializes as T. The registered signal is wired at flow
// startup: a per-signal goroutine receives from the workflow signal channel
// directly into a value of type T (so Temporal's data converter does the
// typed deserialization), then injects it into FlowState's buffer for
// non-blocking consumption via TakeSignal[T] / TakeAllSignals[T] /
// PeekSignal[T].
//
// Multiple signals with different payload types may be registered on the
// same flow. Calling RegisterSignal twice with the same name appends a
// second receive pump for that name — last write wins on conflicts; prefer
// to register each name once.
//
// Pairs with the typed query helper WithQueryState[S]: both replace the old
// untyped Signals().Inject / Signals().Take APIs with type-parameterised
// access that fails at compile time on mismatch.
func RegisterSignal[T any](b *FlowBuilder, name string) *FlowBuilder {
	b.flow.signals = append(b.flow.signals, SignalDef{
		Name: name,
		pump: func(ctx workflow.Context, ch workflow.ReceiveChannel, fs *FlowState) {
			for {
				var payload T
				ch.Receive(ctx, &payload)
				fs.signals.inject(name, payload)
			}
		},
	})
	return b
}

// TakeSignal removes and returns the oldest buffered payload for `name`,
// typed as T. Returns (zero, false) if no payload is buffered.
//
// Panics if the buffered payload is not assertable to T. By construction
// (RegisterSignal[T] is the only public injection path), this can only
// occur if a signal name was registered with a different T than the caller
// requests — i.e. programmer error, surfaced loudly per Go convention.
func TakeSignal[T any](fs *FlowState, name string) (T, bool) {
	var zero T
	raw, ok := fs.signals.take(name)
	if !ok {
		return zero, false
	}
	return raw.(T), true
}

// TakeAllSignals returns all buffered payloads for `name`, typed as T, and
// clears the buffer. Returns nil when no payloads are buffered.
//
// Panics under the same condition as TakeSignal — caller-provided T must
// match the registered T for `name`.
func TakeAllSignals[T any](fs *FlowState, name string) []T {
	raws, ok := fs.signals.takeAll(name)
	if !ok {
		return nil
	}
	out := make([]T, len(raws))
	for i, r := range raws {
		out[i] = r.(T)
	}
	return out
}

// PeekSignal returns the oldest buffered payload for `name`, typed as T,
// without removing it. Returns (zero, false) if no payload is buffered.
//
// Panics under the same condition as TakeSignal.
func PeekSignal[T any](fs *FlowState, name string) (T, bool) {
	var zero T
	raw, ok := fs.signals.peek(name)
	if !ok {
		return zero, false
	}
	return raw.(T), true
}

// SignalCount returns the number of buffered payloads for `name`.
func SignalCount(fs *FlowState, name string) int {
	return fs.signals.size(name)
}

// InjectSignal appends a typed payload directly to the buffer. Used by tests
// to simulate signal arrival without driving the workflow signal channel.
//
// Production code should never call this — production payloads enter the
// buffer via the receive pump installed by RegisterSignal[T].
func InjectSignal[T any](fs *FlowState, name string, payload T) {
	fs.signals.inject(name, payload)
}
