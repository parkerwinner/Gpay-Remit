package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Gpay-Remit/gpay-remit/metrics"
	"github.com/redis/go-redis/v9"
)

var (
	RedisClient       *redis.Client
	ctx               = context.Background()
	evictionMu        sync.Mutex
	lastEvicted       uint64
	hasEvictionSample bool
)

const DefaultCacheTTL = 5 * time.Minute

// ErrCacheUnavailable is returned when Redis is not available
var ErrCacheUnavailable = errors.New("cache service unavailable")

// InitRedis initializes the Redis client with connection pool configuration (#202)
func InitRedis(addr string, password string, db int) error {
	// Configure connection pool for better resource management
	RedisClient = redis.NewClient(&redis.Options{
		Addr:            addr,
		Password:        password,
		DB:              db,
		PoolSize:        10,              // Maximum number of connections
		MinIdleConns:    2,               // Minimum idle connections to maintain
		ConnMaxIdleTime: 5 * time.Minute, // Close idle connections after this time
		DialTimeout:     5 * time.Second, // Connection timeout
		ReadTimeout:     3 * time.Second, // Read timeout
		WriteTimeout:    3 * time.Second, // Write timeout
	})

	// Test connection with short timeout - don't panic if Redis is unavailable
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err := RedisClient.Ping(pingCtx).Result()
	if err != nil {
		// Log warning but continue - application should function without Redis
		fmt.Printf("WARNING: Redis unavailable at startup: %v — caching disabled\n", err)
		RedisClient = nil // Set to nil to trigger graceful degradation
		return nil        // Don't return error - this is graceful degradation
	}

	fmt.Println("INFO: Redis connection established successfully")
	return nil
}

// GetCached retrieves a value from the cache and unmarshals it into dest
// Returns cache miss (false, nil) when Redis is unavailable for graceful degradation (#202)
func GetCached(key string, dest interface{}) (bool, error) {
	if RedisClient == nil {
		metrics.CacheOperationsTotal.WithLabelValues("get", "unavailable").Inc()
		// Redis unavailable - return cache miss to fall back to source of truth
		return false, nil
	}

	val, err := RedisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		metrics.CacheOperationsTotal.WithLabelValues("get", "miss").Inc()
		return false, nil
	} else if err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("get", "error").Inc()
		// Redis error - log at debug level and return cache miss for graceful degradation
		fmt.Printf("DEBUG: Redis Get error for key %s: %v\n", key, err)
		return false, nil
	}

	err = json.Unmarshal([]byte(val), dest)
	if err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("get", "error").Inc()
		return false, err
	}

	metrics.CacheOperationsTotal.WithLabelValues("get", "hit").Inc()
	return true, nil
}

// SetCached stores a value in the cache with a TTL
// Silently continues when Redis is unavailable for graceful degradation (#202)
func SetCached(key string, value interface{}, ttl time.Duration) error {
	if RedisClient == nil {
		metrics.CacheOperationsTotal.WithLabelValues("set", "unavailable").Inc()
		// Redis unavailable - log at debug level and continue
		fmt.Printf("DEBUG: Redis unavailable, skipping cache set for key %s\n", key)
		return nil
	}
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}

	data, err := json.Marshal(value)
	if err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("set", "error").Inc()
		return err
	}

	err = RedisClient.Set(ctx, key, data, ttl).Err()
	if err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("set", "error").Inc()
		// Redis error - log at debug level and continue (don't propagate error)
		fmt.Printf("DEBUG: Redis Set error for key %s: %v\n", key, err)
		return nil
	}

	metrics.CacheOperationsTotal.WithLabelValues("set", "success").Inc()
	return nil
}

// DeleteCached removes a value from the cache
// Silently continues when Redis is unavailable for graceful degradation (#202)
func DeleteCached(key string) error {
	if RedisClient == nil {
		metrics.CacheOperationsTotal.WithLabelValues("delete", "unavailable").Inc()
		// Redis unavailable - log at debug level and continue
		fmt.Printf("DEBUG: Redis unavailable, skipping cache delete for key %s\n", key)
		return nil
	}

	err := RedisClient.Del(ctx, key).Err()
	if err != nil {
		metrics.CacheOperationsTotal.WithLabelValues("delete", "error").Inc()
		// Redis error - log at debug level and continue (don't propagate error)
		fmt.Printf("DEBUG: Redis Del error for key %s: %v\n", key, err)
		return nil
	}

	metrics.CacheOperationsTotal.WithLabelValues("delete", "success").Inc()
	return nil
}

// StartRedisStatsCollector samples Redis' cumulative eviction count so the
// exported counter can be alerted on with rate(gpay_remit_redis_evicted_keys_total[5m]).
func StartRedisStatsCollector(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		collectRedisEvictions(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				collectRedisEvictions(ctx)
			}
		}
	}()
}

func collectRedisEvictions(parent context.Context) {
	if RedisClient == nil {
		return
	}
	infoCtx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	info, err := RedisClient.Info(infoCtx, "stats").Result()
	if err != nil {
		return
	}
	var evicted uint64
	for _, line := range strings.Split(info, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && key == "evicted_keys" {
			evicted, err = strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err != nil {
				return
			}
			break
		}
	}

	evictionMu.Lock()
	defer evictionMu.Unlock()
	if !hasEvictionSample || evicted < lastEvicted {
		metrics.RedisEvictedKeysTotal.Add(float64(evicted))
	} else if evicted > lastEvicted {
		metrics.RedisEvictedKeysTotal.Add(float64(evicted - lastEvicted))
	}
	lastEvicted = evicted
	hasEvictionSample = true
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

// Cache is the key/value cache surface used by services that need to be
// testable without a live Redis. RedisCache is the production implementation.
type Cache interface {
	Get(key string, dest interface{}) (bool, error)
	Set(key string, value interface{}, ttl time.Duration) error
}

// RedisCache adapts the package-level helpers above to the Cache interface.
// It degrades to cache misses / no-ops when Redis is unavailable.
type RedisCache struct{}

// Get implements Cache.
func (RedisCache) Get(key string, dest interface{}) (bool, error) {
	return GetCached(key, dest)
}

// Set implements Cache.
func (RedisCache) Set(key string, value interface{}, ttl time.Duration) error {
	return SetCached(key, value, ttl)
}
