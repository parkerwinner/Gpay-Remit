package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	RedisClient *redis.Client
	ctx         = context.Background()
)

// InitRedis initializes the Redis client
func InitRedis(addr string, password string, db int) error {
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	return nil
}

// GetCached retrieves a value from the cache and unmarshals it into dest
func GetCached(key string, dest interface{}) (bool, error) {
	if RedisClient == nil {
		return false, nil
	}

	val, err := RedisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	} else if err != nil {
		return false, err
	}

	err = json.Unmarshal([]byte(val), dest)
	if err != nil {
		return false, err
	}

	return true, nil
}

// SetCached stores a value in the cache with a TTL
func SetCached(key string, value interface{}, ttl time.Duration) error {
	if RedisClient == nil {
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return RedisClient.Set(ctx, key, data, ttl).Err()
}

// DeleteCached removes a value from the cache
func DeleteCached(key string) error {
	if RedisClient == nil {
		return nil
	}

	return RedisClient.Del(ctx, key).Err()
}
