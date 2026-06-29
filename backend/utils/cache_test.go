package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetRedisClient sets RedisClient to nil before each test so tests are
// isolated regardless of execution order.
func resetRedisClient() {
	RedisClient = nil
}

// TestGetCachedNilClient verifies that GetCached is a no-op when the Redis
// client has not been initialised (returns false, nil).
func TestGetCachedNilClient(t *testing.T) {
	resetRedisClient()

	var dest map[string]interface{}
	found, err := GetCached("some-key", &dest)

	assert.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, dest)
}

// TestSetCachedNilClient verifies that SetCached silently succeeds (no-op)
// when the Redis client is nil.
func TestSetCachedNilClient(t *testing.T) {
	resetRedisClient()

	err := SetCached("some-key", map[string]string{"hello": "world"}, time.Minute)

	assert.NoError(t, err)
}

// TestDeleteCachedNilClient verifies that DeleteCached silently succeeds
// (no-op) when the Redis client is nil.
func TestDeleteCachedNilClient(t *testing.T) {
	resetRedisClient()

	err := DeleteCached("some-key")

	assert.NoError(t, err)
}

// TestGetCachedNotFound verifies that GetCached returns (false, nil) for a key
// that has never been stored (requires a live Redis connection; skipped when
// REDIS_ADDR is not reachable).
func TestGetCachedNotFound(t *testing.T) {
	if RedisClient == nil {
		t.Skip("skipping: no Redis client available")
	}

	var dest string
	found, err := GetCached("nonexistent-key-xyz", &dest)

	require.NoError(t, err)
	assert.False(t, found)
}

// TestSetAndGetCached verifies that a value stored with SetCached can be
// retrieved with GetCached (requires a live Redis connection).
func TestSetAndGetCached(t *testing.T) {
	if RedisClient == nil {
		t.Skip("skipping: no Redis client available")
	}

	type payload struct {
		Message string `json:"message"`
	}

	key := "test-set-get-key"
	want := payload{Message: "hello-redis"}

	require.NoError(t, SetCached(key, want, 10*time.Second))
	t.Cleanup(func() { _ = DeleteCached(key) })

	var got payload
	found, err := GetCached(key, &got)

	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, want, got)
}

// TestDeleteCached verifies that after a key is deleted GetCached returns false
// (requires a live Redis connection).
func TestDeleteCached(t *testing.T) {
	if RedisClient == nil {
		t.Skip("skipping: no Redis client available")
	}

	key := "test-delete-key"
	require.NoError(t, SetCached(key, "value", 10*time.Second))

	require.NoError(t, DeleteCached(key))

	var dest string
	found, err := GetCached(key, &dest)
	require.NoError(t, err)
	assert.False(t, found)
}
