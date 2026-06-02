// Package storage defines the key-value store interface and implementations.
package storage

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a key does not exist or has expired.
var ErrNotFound = errors.New("storage: key not found")

// Store is the key-value storage interface used by tools.
type Store interface {
	// Set stores a key-value pair. If ttl is 0, the entry never expires.
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	// Get retrieves a value. Returns ErrNotFound if the key is absent or expired.
	Get(ctx context.Context, key string) (string, error)
	// Delete removes a key.
	Delete(ctx context.Context, key string) error
	// List returns all keys with the given prefix.
	List(ctx context.Context, prefix string) ([]string, error)
	// Close releases resources.
	Close() error
}
