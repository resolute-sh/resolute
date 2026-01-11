package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// DataRef is a reference to data stored in an external storage backend.
// It enables the Claim Check pattern for passing large data between activities
// without bloating Temporal's workflow history.
type DataRef struct {
	StorageKey string    `json:"storage_key"`
	Schema     string    `json:"schema"`
	Count      int       `json:"count"`
	Checksum   string    `json:"checksum,omitempty"`
	Backend    string    `json:"backend"`
	CreatedAt  time.Time `json:"created_at"`
}

// Backend identifiers
const (
	BackendLocal = "local"
	BackendS3    = "s3"
	BackendGCS   = "gcs"
)

// NewDataRef creates a new DataRef with the given parameters.
func NewDataRef(storageKey, schema, backend string, count int) DataRef {
	return DataRef{
		StorageKey: storageKey,
		Schema:     schema,
		Backend:    backend,
		Count:      count,
		CreatedAt:  time.Now(),
	}
}

// WithChecksum adds a checksum to the DataRef for content verification.
func (r DataRef) WithChecksum(data []byte) DataRef {
	hash := sha256.Sum256(data)
	r.Checksum = hex.EncodeToString(hash[:])
	return r
}

// IsEmpty returns true if the DataRef has no storage key.
func (r DataRef) IsEmpty() bool {
	return r.StorageKey == ""
}

// Validate checks that the DataRef has required fields.
func (r DataRef) Validate() error {
	if r.StorageKey == "" {
		return fmt.Errorf("dataref: storage_key is required")
	}
	if r.Schema == "" {
		return fmt.Errorf("dataref: schema is required")
	}
	if r.Backend == "" {
		return fmt.Errorf("dataref: backend is required")
	}
	return nil
}

// String returns a human-readable representation of the DataRef.
func (r DataRef) String() string {
	return fmt.Sprintf("DataRef{backend=%s, key=%s, schema=%s, count=%d}",
		r.Backend, r.StorageKey, r.Schema, r.Count)
}
