package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/resolute-sh/resolute/core"
)

const (
	// DefaultDir is the default directory for local state files.
	DefaultDir = ".resolute"
)

// LocalBackend stores state in local JSON files.
// Default location: .resolute/{flow-name}.json
type LocalBackend struct {
	dir string
	mu  sync.RWMutex
}

// Local creates a local file backend using the default .resolute/ directory.
//
// Example:
//
//	flow := core.NewFlow("my-flow").
//	    WithState(state.NewConfig(state.Local())).
//	    // ...
func Local() *LocalBackend {
	return &LocalBackend{dir: DefaultDir}
}

// LocalWithPath creates a local file backend with a custom directory.
//
// Example:
//
//	flow := core.NewFlow("my-flow").
//	    WithState(state.NewConfig(state.LocalWithPath("/var/resolute/state"))).
//	    // ...
func LocalWithPath(dir string) *LocalBackend {
	return &LocalBackend{dir: dir}
}

// Load reads persisted state from a local JSON file.
func (b *LocalBackend) Load(workflowID, flowName string) (*core.PersistedState, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	path := b.filePath(flowName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No previous state
		}
		return nil, fmt.Errorf("read state file %s: %w", path, err)
	}

	var state core.PersistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	return &state, nil
}

// Save writes persisted state to a local JSON file.
func (b *LocalBackend) Save(workflowID, flowName string, state *core.PersistedState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Ensure directory exists
	if err := os.MkdirAll(b.dir, 0755); err != nil {
		return fmt.Errorf("create state directory %s: %w", b.dir, err)
	}

	// Increment version
	state.Version++

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	path := b.filePath(flowName)

	// Write atomically using temp file + rename
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("write temp state file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("rename state file: %w", err)
	}

	return nil
}

// filePath returns the full path to the state file for a flow.
func (b *LocalBackend) filePath(flowName string) string {
	return filepath.Join(b.dir, flowName+".json")
}

// Dir returns the directory where state files are stored.
func (b *LocalBackend) Dir() string {
	return b.dir
}

// Delete removes the state file for a flow.
func (b *LocalBackend) Delete(flowName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	path := b.filePath(flowName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete state file %s: %w", path, err)
	}
	return nil
}

// List returns all flow names that have persisted state.
func (b *LocalBackend) List() ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entries, err := os.ReadDir(b.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state directory: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			names = append(names, name[:len(name)-5])
		}
	}

	return names, nil
}

// init sets up the default local backend.
func init() {
	core.SetDefaultBackend(Local())
}
