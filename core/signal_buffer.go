package core

import (
	"sync"

	"go.temporal.io/sdk/workflow"
)

// SignalDef is the internal record of a signal registration. Construct via
// RegisterSignal[T] — the typed registration helper that wires the receive
// pump and ensures payloads enter the buffer with a known type.
type SignalDef struct {
	Name string
	pump func(workflow.Context, workflow.ReceiveChannel, *FlowState)
}

// SignalBuffer holds buffered signal payloads per signal name. Storage is
// untyped at the map level because signals with different payload types
// share the buffer; type safety is enforced at registration (RegisterSignal[T])
// and recovered at consumption (TakeSignal[T] / TakeAllSignals[T] /
// PeekSignal[T]).
//
// Constructed once per FlowState by NewFlowState. Direct construction is not
// part of the public API; callers reach the buffer through the typed
// accessors above.
type SignalBuffer struct {
	mu      sync.RWMutex
	buffers map[string][]interface{}
	maxSize int
}

// signalBufferOption configures a SignalBuffer at construction.
type signalBufferOption func(*SignalBuffer)

// withMaxSize sets the maximum buffered signals per name. When exceeded,
// oldest entries are dropped. Default: 100.
func withMaxSize(n int) signalBufferOption {
	return func(sb *SignalBuffer) { sb.maxSize = n }
}

// newSignalBuffer creates a new SignalBuffer with default settings.
func newSignalBuffer(opts ...signalBufferOption) *SignalBuffer {
	sb := &SignalBuffer{
		buffers: make(map[string][]interface{}),
		maxSize: 100,
	}
	for _, opt := range opts {
		opt(sb)
	}
	return sb
}

// take removes and returns the oldest buffered signal for the given name.
// Returns (value, true) if buffered, (nil, false) otherwise.
func (sb *SignalBuffer) take(name string) (interface{}, bool) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	q, ok := sb.buffers[name]
	if !ok || len(q) == 0 {
		return nil, false
	}
	val := q[0]
	if len(q) == 1 {
		delete(sb.buffers, name)
	} else {
		sb.buffers[name] = q[1:]
	}
	return val, true
}

// takeAll returns all buffered signals for the given name and clears the
// buffer.
func (sb *SignalBuffer) takeAll(name string) ([]interface{}, bool) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	q, ok := sb.buffers[name]
	if !ok || len(q) == 0 {
		return nil, false
	}
	delete(sb.buffers, name)
	return q, true
}

// peek returns the oldest buffered signal without removing it.
func (sb *SignalBuffer) peek(name string) (interface{}, bool) {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	q, ok := sb.buffers[name]
	if !ok || len(q) == 0 {
		return nil, false
	}
	return q[0], true
}

// size returns the number of buffered signals for the given name.
func (sb *SignalBuffer) size(name string) int {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	q, ok := sb.buffers[name]
	if !ok {
		return 0
	}
	return len(q)
}

// inject appends a signal payload to the buffer. Called by the typed receive
// pump installed by RegisterSignal[T] and by tests via the typed Inject
// helpers.
func (sb *SignalBuffer) inject(name string, payload interface{}) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	q := sb.buffers[name]
	if len(q) >= sb.maxSize && sb.maxSize > 0 {
		q = q[1:] // drop oldest
	}
	sb.buffers[name] = append(q, payload)
}
