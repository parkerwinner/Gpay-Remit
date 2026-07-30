package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	RedisClient *redis.Client
	ctx         = context.Background()
)

// ErrCacheUnavailable is returned when Redis is not available
var ErrCacheUnavailable = errors.New("cache service unavailable")

// InitRedis initializes the Redis client with connection pool configuration (#202)
func InitRedis(addr string, password string, db int) error {
	// Configure connection pool for better resource management
	RedisClient = redis.NewClient(&redis.Options{
		Addr:            addr,
		Password:        password,
		DB:              db,
		PoolSize:        10,               // Maximum number of connections
		MinIdleConns:    2,                // Minimum idle connections to maintain
		ConnMaxIdleTime: 5 * time.Minute,  // Close idle connections after this time
		DialTimeout:     5 * time.Second,  // Connection timeout
		ReadTimeout:     3 * time.Second,  // Read timeout
		WriteTimeout:    3 * time.Second,  // Write timeout
	})

	// Test connection with short timeout - don't panic if Redis is unavailable
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	
	_, err := RedisClient.Ping(pingCtx).Result()
	if err != nil {
		// Log warning but continue - application should function without Redis
		fmt.Printf("WARNING: Redis unavailable at startup: %v — caching disabled\n", err)
		RedisClient = nil // Set to nil to trigger graceful degradation
		return nil // Don't return error - this is graceful degradation
	}

	fmt.Println("INFO: Redis connection established successfully")
	return nil
}

// GetCached retrieves a value from the cache and unmarshals it into dest
// Returns cache miss (false, nil) when Redis is unavailable for graceful degradation (#202)
func GetCached(key string, dest interface{}) (bool, error) {
	if RedisClient == nil {
		// Redis unavailable - return cache miss to fall back to source of truth
		return false, nil
	}

	val, err := RedisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	} else if err != nil {
		// Redis error - log at debug level and return cache miss for graceful degradation
		fmt.Printf("DEBUG: Redis Get error for key %s: %v\n", key, err)
		return false, nil
	}

	err = json.Unmarshal([]byte(val), dest)
	if err != nil {
		return false, err
	}

	return true, nil
}

// SetCached stores a value in the cache with a TTL
// Silently continues when Redis is unavailable for graceful degradation (#202)
func SetCached(key string, value interface{}, ttl time.Duration) error {
	if RedisClient == nil {
		// Redis unavailable - log at debug level and continue
		fmt.Printf("DEBUG: Redis unavailable, skipping cache set for key %s\n", key)
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	err = RedisClient.Set(ctx, key, data, ttl).Err()
	if err != nil {
		// Redis error - log at debug level and continue (don't propagate error)
		fmt.Printf("DEBUG: Redis Set error for key %s: %v\n", key, err)
		return nil
	}

	return nil
}

// DeleteCached removes a value from the cache
// Silently continues when Redis is unavailable for graceful degradation (#202)
func DeleteCached(key string) error {
	if RedisClient == nil {
		// Redis unavailable - log at debug level and continue
		fmt.Printf("DEBUG: Redis unavailable, skipping cache delete for key %s\n", key)
		return nil
	}

	err := RedisClient.Del(ctx, key).Err()
	if err != nil {
		// Redis error - log at debug level and continue (don't propagate error)
		fmt.Printf("DEBUG: Redis Del error for key %s: %v\n", key, err)
		return nil
	}

	return nil
}

// PingRedis checks Redis availability for health checks
// Returns ErrCacheUnavailable when Redis is not available (#202)
func PingRedis() error {
	if RedisClient == nil {
		return ErrCacheUnavailable
	}

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	
	_, err := RedisClient.Ping(pingCtx).Result()
	if err != nil {
		return ErrCacheUnavailable
	}

	return nil
}
