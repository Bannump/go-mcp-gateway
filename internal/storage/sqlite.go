package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore is a SQLite-backed implementation of Store using WAL mode.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at path and initialises the schema.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("storage: sqlite open: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS kv_store (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			expires_at INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return nil, fmt.Errorf("storage: sqlite schema: %w", err)
	}

	s := &SQLiteStore{db: db}
	go s.sweepExpired()
	return s, nil
}

func (s *SQLiteStore) sweepExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now().Unix()
		s.db.Exec(`DELETE FROM kv_store WHERE expires_at > 0 AND expires_at <= ?`, now) //nolint:errcheck
	}
}

// Set stores a key-value pair. TTL of 0 means no expiry (stored as 0).
func (s *SQLiteStore) Set(_ context.Context, key, value string, ttl time.Duration) error {
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).Unix()
	}
	_, err := s.db.Exec(
		`INSERT INTO kv_store(key, value, expires_at) VALUES(?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, expires_at=excluded.expires_at`,
		key, value, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("storage: sqlite set: %w", err)
	}
	return nil
}

// Get retrieves a value, returning ErrNotFound if missing or expired.
func (s *SQLiteStore) Get(_ context.Context, key string) (string, error) {
	var value string
	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT value, expires_at FROM kv_store WHERE key = ?`, key,
	).Scan(&value, &expiresAt)

	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("storage: sqlite get: %w", err)
	}
	if expiresAt > 0 && time.Now().Unix() >= expiresAt {
		return "", ErrNotFound
	}
	return value, nil
}

// Delete removes a key.
func (s *SQLiteStore) Delete(_ context.Context, key string) error {
	if _, err := s.db.Exec(`DELETE FROM kv_store WHERE key = ?`, key); err != nil {
		return fmt.Errorf("storage: sqlite delete: %w", err)
	}
	return nil
}

// List returns all non-expired keys with the given prefix.
func (s *SQLiteStore) List(_ context.Context, prefix string) ([]string, error) {
	now := time.Now().Unix()
	rows, err := s.db.Query(
		`SELECT key FROM kv_store WHERE key LIKE ? AND (expires_at = 0 OR expires_at > ?)`,
		prefix+"%", now,
	)
	if err != nil {
		return nil, fmt.Errorf("storage: sqlite list: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("storage: sqlite list scan: %w", err)
		}
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, rows.Err()
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Healthy returns true if the database is reachable.
func (s *SQLiteStore) Healthy() bool {
	return s.db.Ping() == nil
}
