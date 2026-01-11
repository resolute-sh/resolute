package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// StorageBackend defines the interface for storing and retrieving data
// referenced by DataRef. Implementations handle the actual persistence.
type StorageBackend interface {
	// Store saves data and returns a DataRef pointing to it.
	Store(ctx context.Context, schema string, data []byte) (DataRef, error)

	// Load retrieves data by its DataRef.
	Load(ctx context.Context, ref DataRef) ([]byte, error)

	// Delete removes data referenced by DataRef.
	Delete(ctx context.Context, ref DataRef) error

	// Backend returns the backend identifier.
	Backend() string
}

// Storage provides typed helpers for storing and loading data.
type Storage struct {
	backend StorageBackend
}

// NewStorage creates a new Storage with the given backend.
func NewStorage(backend StorageBackend) *Storage {
	return &Storage{backend: backend}
}

// StoreJSON stores any JSON-serializable data and returns a DataRef.
func (s *Storage) StoreJSON(ctx context.Context, schema string, v interface{}) (DataRef, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return DataRef{}, fmt.Errorf("marshal: %w", err)
	}

	ref, err := s.backend.Store(ctx, schema, data)
	if err != nil {
		return DataRef{}, err
	}

	return ref.WithChecksum(data), nil
}

// LoadJSON loads JSON data from a DataRef into the provided destination.
func (s *Storage) LoadJSON(ctx context.Context, ref DataRef, dest interface{}) error {
	data, err := s.backend.Load(ctx, ref)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	return nil
}

// Delete removes data referenced by DataRef.
func (s *Storage) Delete(ctx context.Context, ref DataRef) error {
	return s.backend.Delete(ctx, ref)
}

// LocalStorage implements StorageBackend using the local filesystem.
type LocalStorage struct {
	basePath string
	mu       sync.RWMutex
}

// NewLocalStorage creates a LocalStorage with the given base path.
func NewLocalStorage(basePath string) (*LocalStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}
	return &LocalStorage{basePath: basePath}, nil
}

// Backend returns the backend identifier.
func (s *LocalStorage) Backend() string {
	return BackendLocal
}

// Store saves data to a local file and returns a DataRef.
func (s *LocalStorage) Store(ctx context.Context, schema string, data []byte) (DataRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := generateStorageKey()
	filePath := filepath.Join(s.basePath, key+".json")

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return DataRef{}, fmt.Errorf("write file: %w", err)
	}

	return DataRef{
		StorageKey: key,
		Schema:     schema,
		Backend:    BackendLocal,
		CreatedAt:  time.Now(),
	}, nil
}

// Load retrieves data from a local file.
func (s *LocalStorage) Load(ctx context.Context, ref DataRef) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := filepath.Join(s.basePath, ref.StorageKey+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("data not found: %s", ref.StorageKey)
		}
		return nil, fmt.Errorf("read file: %w", err)
	}

	return data, nil
}

// Delete removes a local file.
func (s *LocalStorage) Delete(ctx context.Context, ref DataRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := filepath.Join(s.basePath, ref.StorageKey+".json")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove file: %w", err)
	}

	return nil
}

// generateStorageKey creates a unique storage key.
func generateStorageKey() string {
	return uuid.New().String()
}

// DefaultLocalStorage returns a LocalStorage using .resolute/data directory.
func DefaultLocalStorage() (*LocalStorage, error) {
	return NewLocalStorage(".resolute/data")
}

// Global storage instance for convenience
var (
	globalStorage     *Storage
	globalStorageOnce sync.Once
	globalStorageErr  error
)

// GetStorage returns the global storage instance.
// Initializes with local storage on first call.
func GetStorage() (*Storage, error) {
	globalStorageOnce.Do(func() {
		backend, err := DefaultLocalStorage()
		if err != nil {
			globalStorageErr = err
			return
		}
		globalStorage = NewStorage(backend)
	})

	if globalStorageErr != nil {
		return nil, globalStorageErr
	}
	return globalStorage, nil
}

// SetStorage sets the global storage instance.
// Call this early in initialization to use a custom backend.
func SetStorage(s *Storage) {
	globalStorage = s
	globalStorageErr = nil
}
