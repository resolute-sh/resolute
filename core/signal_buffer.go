package core

import "sync"

// SignalDef declares a signal that a flow accepts for non-blocking buffering.
type SignalDef struct {
	Name string
}

// SignalBuffer holds buffered signal payloads per signal name.
// It is safe for concurrent use.
type SignalBuffer struct {
	mu      sync.RWMutex
	buffers map[string][]interface{}
	maxSize int
}

// SignalBufferOption configures a SignalBuffer.
type SignalBufferOption func(*SignalBuffer)

// WithMaxSize sets the maximum buffered signals per name.
// When exceeded, oldest entries are dropped. Default: 100.
func WithMaxSize(n int) SignalBufferOption {
	return func(sb *SignalBuffer) { sb.maxSize = n }
}

// NewSignalBuffer creates a new SignalBuffer with default settings.
func NewSignalBuffer(opts ...SignalBufferOption) *SignalBuffer {
	sb := &SignalBuffer{
		buffers: make(map[string][]interface{}),
		maxSize: 100,
	}
	for _, opt := range opts {
		opt(sb)
	}
	return sb
}

// Take removes and returns the oldest buffered signal for the given name.
// Returns (value, true) if buffered, (nil, false) otherwise.
func (sb *SignalBuffer) Take(name string) (interface{}, bool) {
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

// TakeAll returns all buffered signals for the given name and clears the buffer.
func (sb *SignalBuffer) TakeAll(name string) ([]interface{}, bool) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	q, ok := sb.buffers[name]
	if !ok || len(q) == 0 {
		return nil, false
	}
	delete(sb.buffers, name)
	return q, true
}

// Peek returns the oldest buffered signal without removing it.
func (sb *SignalBuffer) Peek(name string) (interface{}, bool) {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	q, ok := sb.buffers[name]
	if !ok || len(q) == 0 {
		return nil, false
	}
	return q[0], true
}

// Len returns the number of buffered signals for the given name.
func (sb *SignalBuffer) Len(name string) int {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	q, ok := sb.buffers[name]
	if !ok {
		return 0
	}
	return len(q)
}

// Inject appends a signal payload to the buffer. Used by signal handlers
// and in tests to simulate signal arrival.
func (sb *SignalBuffer) Inject(name string, payload interface{}) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	q := sb.buffers[name]
	if len(q) >= sb.maxSize && sb.maxSize > 0 {
		q = q[1:] // drop oldest
	}
	sb.buffers[name] = append(q, payload)
}
