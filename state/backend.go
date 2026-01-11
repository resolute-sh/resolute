// Package state provides state persistence backends for resolute workflows.
package state

import (
	"github.com/resolute-sh/resolute/core"
)

// Backend is an alias for core.StateBackend for convenience.
type Backend = core.StateBackend

// Config is an alias for core.StateConfig for convenience.
type Config = core.StateConfig

// PersistedState is an alias for core.PersistedState for convenience.
type PersistedState = core.PersistedState

// Cursor is an alias for core.Cursor for convenience.
type Cursor = core.Cursor

// NewConfig creates a StateConfig with the given backend.
func NewConfig(backend Backend) Config {
	return Config{Backend: backend}
}

// NewConfigWithNamespace creates a StateConfig with backend and namespace.
func NewConfigWithNamespace(backend Backend, namespace string) Config {
	return Config{
		Backend:   backend,
		Namespace: namespace,
	}
}
