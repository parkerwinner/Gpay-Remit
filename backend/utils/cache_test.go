package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test graceful degradation when Redis is unavailable (#202)
func TestRedisGracefulDegradation(t *testing.T) {
	// Save original client
	originalClient := RedisClient
	defer func() {
		RedisClient = originalClient
	}()

	t.Run("GetCached returns cache miss when Redis unavailable", func(t *testing.T) {
		// Set client to nil to simulate unavailability
		RedisClient = nil
		
		var result interface{}
		found, err := GetCached("test-key", &result)
		
		assert.False(t, found)
		assert.NoError(t, err)
	})

	t.Run("SetCached continues silently when Redis unavailable", func(t *testing.T) {
		// Set client to nil to simulate unavailability
		RedisClient = nil
		
		err := SetCached("test-key", map[string]string{"test": "data"}, time.Minute)
		
		// Should not return error for graceful degradation
		assert.NoError(t, err)
	})

	t.Run("DeleteCached continues silently when Redis unavailable", func(t *testing.T) {
		// Set client to nil to simulate unavailability
		RedisClient = nil
		
		err := DeleteCached("test-key")
		
		// Should not return error for graceful degradation
		assert.NoError(t, err)
	})

	t.Run("PingRedis returns ErrCacheUnavailable when Redis unavailable", func(t *testing.T) {
		// Set client to nil to simulate unavailability
		RedisClient = nil
		
		err := PingRedis()
		
		assert.Equal(t, ErrCacheUnavailable, err)
	})
}

// Test that InitRedis doesn't panic when Redis is unavailable (#202)
func TestInitRedisGracefulFailure(t *testing.T) {
	t.Run("InitRedis handles connection failure gracefully", func(t *testing.T) {
		// Try to connect to non-existent Redis instance
		err := InitRedis("localhost:9999", "", 0)
		
		// Should not return error (graceful degradation)
		assert.NoError(t, err)
		
		// Client should be nil after failed initialization
		assert.Nil(t, RedisClient)
	})
}

// Test cache operations with working Redis (integration test)
func TestCacheOperationsWithRedis(t *testing.T) {
	// This test requires a running Redis instance
	// Skip if Redis is not available
	err := InitRedis("localhost:6379", "", 0)
	if err != nil || RedisClient == nil {
		t.Skip("Redis not available for integration test")
	}

	t.Run("Set and Get cache value", func(t *testing.T) {
		testData := map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		}
		
		err := SetCached("integration-test-key", testData, time.Minute)
		assert.NoError(t, err)
		
		var result map[string]interface{}
		found, err := GetCached("integration-test-key", &result)
		
		assert.True(t, found)
		assert.NoError(t, err)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, float64(123), result["key2"]) // JSON unmarshaling converts numbers to float64
		
		// Clean up
		err = DeleteCached("integration-test-key")
		assert.NoError(t, err)
	})

	t.Run("Get non-existent key returns cache miss", func(t *testing.T) {
		var result interface{}
		found, err := GetCached("non-existent-key", &result)
		
		assert.False(t, found)
		assert.NoError(t, err)
	})

	t.Run("PingRedis succeeds with working Redis", func(t *testing.T) {
		err := PingRedis()
		assert.NoError(t, err)
	})
}

// Test connection pool configuration
func TestRedisConnectionPoolConfig(t *testing.T) {
	// This test verifies that InitRedis sets up connection pool properly
	// We can't easily test the actual pool settings without accessing internals,
	// but we can verify the function completes successfully
	
	t.Run("InitRedis with valid config", func(t *testing.T) {
		// This should not panic and should handle connection gracefully
		assert.NotPanics(t, func() {
			InitRedis("localhost:6379", "", 0)
		})
	})
}