package storage

import (
	"context"
	"strings"
	"sync"
	"time"
)

type entry struct {
	value    string
	expiresAt time.Time // zero means no expiry
}

func (e entry) expired() bool {
	return !e.expiresAt.IsZero() && time.Now().After(e.expiresAt)
}

// MemoryStore is a thread-safe in-memory implementation of Store.
type MemoryStore struct {
	mu      sync.RWMutex
	data    map[string]entry
	stopCh  chan struct{}
}

// NewMemoryStore creates a MemoryStore and starts the background expiry sweep.
func NewMemoryStore() *MemoryStore {
	m := &MemoryStore{
		data:   make(map[string]entry),
		stopCh: make(chan struct{}),
	}
	go m.sweep()
	return m
}

func (m *MemoryStore) sweep() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.mu.Lock()
			for k, e := range m.data {
				if e.expired() {
					delete(m.data, k)
				}
			}
			m.mu.Unlock()
		}
	}
}

// Set stores a key-value pair with an optional TTL.
func (m *MemoryStore) Set(_ context.Context, key, value string, ttl time.Duration) error {
	e := entry{value: value}
	if ttl > 0 {
		e.expiresAt = time.Now().Add(ttl)
	}
	m.mu.Lock()
	m.data[key] = e
	m.mu.Unlock()
	return nil
}

// Get retrieves a value and expiry time, returning ErrNotFound if missing or expired.
// The zero time.Time means no expiry was set.
func (m *MemoryStore) Get(_ context.Context, key string) (string, time.Time, error) {
	m.mu.RLock()
	e, ok := m.data[key]
	m.mu.RUnlock()

	if !ok || e.expired() {
		return "", time.Time{}, ErrNotFound
	}
	return e.value, e.expiresAt, nil
}

// Delete removes a key.
func (m *MemoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
	return nil
}

// List returns all keys with the given prefix.
func (m *MemoryStore) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []string
	for k, e := range m.data {
		if !e.expired() && strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// Close stops the background sweep goroutine.
func (m *MemoryStore) Close() error {
	close(m.stopCh)
	return nil
}
