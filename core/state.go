package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.temporal.io/sdk/workflow"
)

// WindowMeta holds ephemeral window cursor and size for the current batch iteration.
type WindowMeta struct {
	Cursor string
	Size   int
}

// FlowState carries data through workflow execution.
// It holds input data, activity outputs, and cursor state for incremental processing.
type FlowState struct {
	mu      sync.RWMutex
	input   map[string][]byte // Serialized initial input
	// results holds activity outputs. Untyped storage required because activities
	// return heterogeneous types; type safety restored via Get[T]() accessor.
	results    map[string]any
	cursors    map[string]Cursor // Loaded from persisted state
	windowMeta *WindowMeta       // Ephemeral, set by windowed executor per batch
}

// Cursor tracks incremental processing position for a data source.
type Cursor struct {
	Source    string    `json:"source"`
	Position  string    `json:"position"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewFlowState creates a new flow state with the given input.
func NewFlowState(input FlowInput) *FlowState {
	return &FlowState{
		input:   input.Data,
		results: make(map[string]any),
		cursors: make(map[string]Cursor),
	}
}

// GetInputData retrieves raw input data by key.
// Returns the byte slice and whether the key was found.
func (s *FlowState) GetInputData(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.input[key]
	return data, ok
}

// GetResult retrieves a raw result by key. Returns any because activity result
// types vary; prefer typed Get[T]() for compile-time type safety.
func (s *FlowState) GetResult(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.results[key]
}

// SetResult stores a result by key. Accepts any because activity result types
// vary; type verification happens at retrieval via Get[T]().
func (s *FlowState) SetResult(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[key] = value
}

// GetCursor returns the cursor for a data source.
func (s *FlowState) GetCursor(source string) Cursor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if c, ok := s.cursors[source]; ok {
		return c
	}
	return Cursor{Source: source}
}

// SetCursor updates the cursor for a data source.
func (s *FlowState) SetCursor(source, position string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursors[source] = Cursor{
		Source:    source,
		Position:  position,
		UpdatedAt: time.Now(),
	}
}

// SetWindowMeta sets the ephemeral window cursor and size for the current batch.
func (s *FlowState) SetWindowMeta(cursor string, size int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windowMeta = &WindowMeta{Cursor: cursor, Size: size}
}

// GetWindowMeta returns the current window meta. Returns zero WindowMeta if not set.
func (s *FlowState) GetWindowMeta() WindowMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.windowMeta == nil {
		return WindowMeta{}
	}
	return *s.windowMeta
}

// NewBatchState creates an isolated state for a windowed batch.
// Inherits input and cursors from parent. Results and window meta start empty.
func (s *FlowState) NewBatchState() *FlowState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	batch := &FlowState{
		input:   s.input,
		results: make(map[string]any),
		cursors: make(map[string]Cursor),
	}
	for k, v := range s.cursors {
		batch.cursors[k] = v
	}
	return batch
}

// Snapshot creates a copy of the current state for compensation.
func (s *FlowState) Snapshot() *FlowState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := &FlowState{
		input:   make(map[string][]byte),
		results: make(map[string]any),
		cursors: make(map[string]Cursor),
	}

	for k, v := range s.input {
		snapshot.input[k] = v
	}
	for k, v := range s.results {
		snapshot.results[k] = v
	}
	for k, v := range s.cursors {
		snapshot.cursors[k] = v
	}

	return snapshot
}

// LoadPersisted loads persisted state (cursors) from the configured backend.
func (s *FlowState) LoadPersisted(ctx workflow.Context, flowName string, cfg *StateConfig) error {
	backend := getBackend(cfg)
	persisted, err := backend.Load(workflow.GetInfo(ctx).WorkflowExecution.ID, flowName)
	if err != nil {
		return err
	}
	if persisted == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range persisted.Cursors {
		s.cursors[k] = v
	}
	return nil
}

// SavePersisted saves persisted state (cursors) to the configured backend.
func (s *FlowState) SavePersisted(ctx workflow.Context, flowName string, cfg *StateConfig) error {
	s.mu.RLock()
	cursors := make(map[string]Cursor)
	for k, v := range s.cursors {
		cursors[k] = v
	}
	s.mu.RUnlock()

	backend := getBackend(cfg)
	return backend.Save(workflow.GetInfo(ctx).WorkflowExecution.ID, flowName, &PersistedState{
		Cursors:   cursors,
		UpdatedAt: time.Now(),
	})
}

// getBackend returns the configured backend or the default local backend.
func getBackend(cfg *StateConfig) StateBackend {
	if cfg != nil && cfg.Backend != nil {
		return cfg.Backend
	}
	if defaultLocalBackend == nil {
		defaultLocalBackend = newInMemoryBackend()
	}
	return defaultLocalBackend
}

// inMemoryBackend is a no-op state backend used when no backend is configured.
type inMemoryBackend struct{}

func newInMemoryBackend() *inMemoryBackend {
	return &inMemoryBackend{}
}

func (b *inMemoryBackend) Load(workflowID, flowName string) (*PersistedState, error) {
	return nil, nil
}

func (b *inMemoryBackend) Save(workflowID, flowName string, state *PersistedState) error {
	return nil
}

// Time parses the cursor position as a time.Time value.
func (c Cursor) Time() (time.Time, error) {
	if c.Position == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, c.Position)
}

// TimeOr parses the cursor position as time.Time, returning def on error.
func (c Cursor) TimeOr(def time.Time) time.Time {
	t, err := c.Time()
	if err != nil || t.IsZero() {
		return def
	}
	return t
}

// Get retrieves a typed value from results.
// Panics if the key doesn't exist or type doesn't match.
func Get[T any](s *FlowState, key string) T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.results[key]
	if !ok {
		panic("key not found: " + key)
	}

	typed, ok := val.(T)
	if !ok {
		panic("type mismatch for key: " + key)
	}

	return typed
}

// GetOr retrieves a typed value from results with a default fallback.
func GetOr[T any](s *FlowState, key string, defaultVal T) T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.results[key]
	if !ok {
		return defaultVal
	}

	typed, ok := val.(T)
	if !ok {
		return defaultVal
	}

	return typed
}

// Set stores a typed value in results.
func Set[T any](s *FlowState, key string, value T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[key] = value
}

// GetSafe retrieves a typed value from results with error handling.
// Returns an error if the key doesn't exist or type doesn't match.
func GetSafe[T any](s *FlowState, key string) (T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var zero T
	val, ok := s.results[key]
	if !ok {
		return zero, fmt.Errorf("key %q not found in state", key)
	}

	typed, ok := val.(T)
	if !ok {
		return zero, fmt.Errorf("key %q: expected %T, got %T", key, zero, val)
	}

	return typed, nil
}

// MustGet retrieves a typed value from results.
// Panics with a descriptive error if the key doesn't exist or type doesn't match.
func MustGet[T any](s *FlowState, key string) T {
	val, err := GetSafe[T](s, key)
	if err != nil {
		panic(err)
	}
	return val
}

// Has returns true if the key exists in state results.
func Has(s *FlowState, key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.results[key]
	return ok
}

// Keys returns all keys in state results, sorted alphabetically.
func Keys(s *FlowState) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.results))
	for k := range s.results {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// StateConfig defines state persistence behavior.
type StateConfig struct {
	Backend   StateBackend
	Namespace string
}

// StateBackend interface for pluggable state persistence.
type StateBackend interface {
	Load(workflowID, flowName string) (*PersistedState, error)
	Save(workflowID, flowName string, state *PersistedState) error
}

// PersistedState is the data structure saved between workflow runs.
type PersistedState struct {
	Cursors   map[string]Cursor `json:"cursors"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Version   int64             `json:"version"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// MarshalJSON implements json.Marshaler.
func (p *PersistedState) MarshalJSON() ([]byte, error) {
	type Alias PersistedState
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(p),
	})
}

// defaultLocalBackend is the default backend using .resolute/ directory.
var defaultLocalBackend StateBackend

// SetDefaultBackend allows overriding the default backend (for testing).
func SetDefaultBackend(b StateBackend) {
	defaultLocalBackend = b
}
