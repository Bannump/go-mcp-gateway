package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Bannump/go-mcp-gateway/internal/storage"
)

func TestMemoryStore_SetGet(t *testing.T) {
	s := storage.NewMemoryStore()
	defer s.Close()

	require.NoError(t, s.Set(context.Background(), "foo", "bar", 0))
	val, exp, err := s.Get(context.Background(), "foo")
	require.NoError(t, err)
	assert.Equal(t, "bar", val)
	assert.True(t, exp.IsZero(), "no-expiry entry should return zero time")
}

func TestMemoryStore_SetGetWithTTL(t *testing.T) {
	s := storage.NewMemoryStore()
	defer s.Close()

	require.NoError(t, s.Set(context.Background(), "ttlkey", "v", time.Hour))
	_, exp, err := s.Get(context.Background(), "ttlkey")
	require.NoError(t, err)
	assert.False(t, exp.IsZero(), "entry with TTL should return non-zero expiry")
	assert.WithinDuration(t, time.Now().Add(time.Hour), exp, 2*time.Second)
}

func TestMemoryStore_Expired(t *testing.T) {
	s := storage.NewMemoryStore()
	defer s.Close()

	require.NoError(t, s.Set(context.Background(), "key", "value", 10*time.Millisecond))
	time.Sleep(20 * time.Millisecond)

	_, _, err := s.Get(context.Background(), "key")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestMemoryStore_Delete(t *testing.T) {
	s := storage.NewMemoryStore()
	defer s.Close()

	require.NoError(t, s.Set(context.Background(), "key", "value", 0))
	require.NoError(t, s.Delete(context.Background(), "key"))

	_, _, err := s.Get(context.Background(), "key")
	assert.ErrorIs(t, err, storage.ErrNotFound)
}

func TestMemoryStore_ListPrefix(t *testing.T) {
	s := storage.NewMemoryStore()
	defer s.Close()

	require.NoError(t, s.Set(context.Background(), "user:alice", "a", 0))
	require.NoError(t, s.Set(context.Background(), "user:bob", "b", 0))
	require.NoError(t, s.Set(context.Background(), "session:1", "c", 0))

	keys, err := s.List(context.Background(), "user:")
	require.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "user:alice")
	assert.Contains(t, keys, "user:bob")
}
