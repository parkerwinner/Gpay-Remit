package middleware

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/gin-gonic/gin"
)

// RateLimiter stores rate limit information per user
type RateLimiter struct {
	mu              sync.RWMutex
	limits          map[string]*UserLimit
	config          *config.Config
	cleanupInterval time.Duration
	// staleAfter is how long an entry may go unaccessed before cleanup
	// removes it. Separated from cleanupInterval (how often the sweep
	// runs) so tests can exercise cleanup with short, deterministic
	// intervals without changing what "stale" means in production.
	staleAfter time.Duration
	// stop, closed via Stop(), signals the cleanup goroutine to exit —
	// without this there was previously no way to shut the goroutine down
	// (fine for the process-lifetime singleton in production, but it
	// meant a test constructing its own RateLimiter had no way to avoid
	// leaking a goroutine that outlives the test).
	stop chan struct{}
}

// UserLimit tracks request counts for a user
type UserLimit struct {
	Count      int
	ResetAt    time.Time
	LastAccess time.Time
}

// EndpointLimits defines rate limits per endpoint
type EndpointLimits struct {
	Default  int
	Specific map[string]int
}

var (
	globalRateLimiter *RateLimiter
	rateLimiterOnce   sync.Once
)

// defaultCleanupInterval and defaultStaleAfter match this middleware's
// documented behavior (#196): sweep for stale entries every 5 minutes,
// removing anything not accessed in the last hour.
const (
	defaultCleanupInterval = 5 * time.Minute
	defaultStaleAfter      = 1 * time.Hour
)

// GetRateLimiter returns the singleton rate limiter instance
func GetRateLimiter(cfg *config.Config) *RateLimiter {
	rateLimiterOnce.Do(func() {
		globalRateLimiter = newRateLimiter(cfg, defaultCleanupInterval, defaultStaleAfter)
	})
	return globalRateLimiter
}

// newRateLimiter constructs a RateLimiter and starts its cleanup goroutine.
// Exposed (unexported) separately from GetRateLimiter so tests can build
// an independent instance with short cleanupInterval/staleAfter values
// instead of sharing — and being unable to reconfigure — the process-wide
// singleton.
func newRateLimiter(cfg *config.Config, cleanupInterval, staleAfter time.Duration) *RateLimiter {
	if os.Getenv("RATE_LIMIT_BACKEND") == "redis" && utils.RedisClient == nil {
		fmt.Println("WARNING: RATE_LIMIT_BACKEND is set to redis but RedisClient is nil; falling back to in-memory rate limiter")
	}

	rl := &RateLimiter{
		limits:          make(map[string]*UserLimit),
		config:          cfg,
		cleanupInterval: cleanupInterval,
		staleAfter:      staleAfter,
		stop:            make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Stop terminates the cleanup goroutine. Safe to call at most once per
// RateLimiter (matching a channel's close-once semantics) — the process-
// lifetime singleton returned by GetRateLimiter is never expected to call
// this; it exists for tests that construct their own RateLimiter and want
// to avoid leaking the cleanup goroutine past the test.
func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

// cleanup removes stale entries periodically until Stop() is called.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stop:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, limit := range rl.limits {
				if now.Sub(limit.LastAccess) > rl.staleAfter {
					delete(rl.limits, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// GetLimit returns the current limit for a user/endpoint
func (rl *RateLimiter) GetLimit(key string) *UserLimit {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.limits[key]
}

// IncrementAndCheck increments the counter and checks if limit is exceeded
func (rl *RateLimiter) IncrementAndCheck(key string, maxRequests int, window time.Duration) (allowed bool, remaining int, resetAt time.Time) {
	backend := os.Getenv("RATE_LIMIT_BACKEND")
	if backend != "memory" && utils.RedisClient != nil {
		redisKey := fmt.Sprintf("rate_limit:%s", key)
		ctx := context.Background()
		count, err := utils.RedisClient.Incr(ctx, redisKey).Result()
		if err == nil {
			if count == 1 {
				utils.RedisClient.Expire(ctx, redisKey, window)
			}
			ttl, errTTL := utils.RedisClient.TTL(ctx, redisKey).Result()
			if errTTL != nil || ttl < 0 {
				ttl = window
			}
			resetAt = time.Now().Add(ttl)
			if int(count) > maxRequests {
				return false, 0, resetAt
			}
			return true, maxRequests - int(count), resetAt
		}
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	limit, exists := rl.limits[key]

	// If no limit exists or the window has reset, create/reset it
	if !exists || now.After(limit.ResetAt) {
		resetAt = now.Add(window)
		rl.limits[key] = &UserLimit{
			Count:      1,
			ResetAt:    resetAt,
			LastAccess: now,
		}
		return true, maxRequests - 1, resetAt
	}

	// Update last access time
	limit.LastAccess = now

	// Check if limit exceeded
	if limit.Count >= maxRequests {
		return false, 0, limit.ResetAt
	}

	// Increment counter
	limit.Count++
	return true, maxRequests - limit.Count, limit.ResetAt
}

// ResetUserLimit resets the rate limit for a specific user/endpoint (admin function)
func (rl *RateLimiter) ResetUserLimit(key string) {
	if utils.RedisClient != nil {
		ctx := context.Background()
		_ = utils.RedisClient.Del(ctx, fmt.Sprintf("rate_limit:%s", key)).Err()
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.limits, key)
}

// GetAllLimits returns all current limits (admin function)
func (rl *RateLimiter) GetAllLimits() map[string]*UserLimit {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	// Create a copy to avoid race conditions
	copy := make(map[string]*UserLimit)
	for k, v := range rl.limits {
		copy[k] = &UserLimit{
			Count:      v.Count,
			ResetAt:    v.ResetAt,
			LastAccess: v.LastAccess,
		}
	}
	return copy
}

// RateLimitMiddleware applies rate limiting based on user ID
func RateLimitMiddleware(cfg *config.Config) gin.HandlerFunc {
	limiter := GetRateLimiter(cfg)
	
	// Default limits per endpoint (requests per minute)
	endpointLimits := map[string]int{
		"POST /api/v1/remittances":        10,  // 10 remittances per minute
		"POST /api/v1/remittances/create": 10,
		"GET /api/v1/remittances":         60,  // 60 reads per minute
		"POST /api/v1/invoices":           20,
		"POST /api/v1/auth/login":         5,   // 5 login attempts per minute
		"POST /api/v1/auth/register":      3,
		"GET /api/v1/exchange-rates":      30, // 30 lookups per minute (backed by cache)
		"GET /api/v2/exchange-rates":      30,
		"default":                         100, // Default 100 requests per minute
	}

	return func(c *gin.Context) {
		// Get user ID from context (set by JWT middleware)
		userIDInterface, exists := c.Get("userID")
		if !exists {
			// If no user ID, fall back to IP-based rate limiting
			userIDInterface = c.ClientIP()
		}

		userID := fmt.Sprintf("%v", userIDInterface)
		endpoint := fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())
		
		// Create a unique key for this user+endpoint combination
		limitKey := fmt.Sprintf("user:%s:endpoint:%s", userID, endpoint)

		// Get the limit for this endpoint
		maxRequests := endpointLimits["default"]
		if limit, ok := endpointLimits[endpoint]; ok {
			maxRequests = limit
		}

		// Check and increment
		allowed, remaining, resetAt := limiter.IncrementAndCheck(limitKey, maxRequests, time.Minute)

		// Set rate limit headers
		c.Header("X-RateLimit-Limit", strconv.Itoa(maxRequests))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

		if !allowed {
			retryAfter := retryAfterSeconds(time.Until(resetAt))
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			AbortWithError(c, http.StatusTooManyRequests, errors.CodeRateLimited, "rate limit exceeded", gin.H{
				"retry_after": retryAfter,
				"reset_at":    resetAt.Format(time.RFC3339),
			})
			return
		}

		c.Next()
	}
}

// retryAfterSeconds converts the time remaining until a rate-limit window resets
// into the whole-second value sent in the Retry-After header. It rounds up so a
// client never retries before the window has reset, and it is at least 1 so a
// 429 never advertises "0" (truncating a sub-second wait used to do that).
func retryAfterSeconds(remaining time.Duration) int {
	secs := int(math.Ceil(remaining.Seconds()))
	if secs < 1 {
		return 1
	}
	return secs
}

// RateLimitMiddleWare is the old function name kept for backward compatibility
func RateLimitMiddleWare() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Mock rate limiter - kept for tests that don't need real rate limiting
		c.Header("X-RateLimit-Limit", "100")
		c.Header("X-RateLimit-Remaining", "99")
		c.Next()
	}
}

// AdminResetRateLimit resets rate limit for a specific user (admin endpoint handler)
func AdminResetRateLimit(cfg *config.Config) gin.HandlerFunc {
	limiter := GetRateLimiter(cfg)
	
	return func(c *gin.Context) {
		userID := c.Query("user_id")
		endpoint := c.Query("endpoint")
		
		if userID == "" {
			AbortWithError(c, http.StatusBadRequest, errors.CodeValidation, "user_id is required", nil)
			return
		}

		if endpoint != "" {
			// Reset specific endpoint for user
			limitKey := fmt.Sprintf("user:%s:endpoint:%s", userID, endpoint)
			limiter.ResetUserLimit(limitKey)
			c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Rate limit reset for user %s on endpoint %s", userID, endpoint)})
		} else {
			// Reset all endpoints for user
			allLimits := limiter.GetAllLimits()
			prefix := fmt.Sprintf("user:%s:", userID)
			count := 0
			for key := range allLimits {
				if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
					limiter.ResetUserLimit(key)
					count++
				}
			}
			c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Reset %d rate limits for user %s", count, userID)})
		}
	}
}

// AdminViewRateLimits returns current rate limit status (admin endpoint handler)
func AdminViewRateLimits(cfg *config.Config) gin.HandlerFunc {
	limiter := GetRateLimiter(cfg)
	
	return func(c *gin.Context) {
		userID := c.Query("user_id")
		
		allLimits := limiter.GetAllLimits()
		
		if userID != "" {
			// Filter by user
			userLimits := make(map[string]*UserLimit)
			prefix := fmt.Sprintf("user:%s:", userID)
			for key, limit := range allLimits {
				if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
					userLimits[key] = limit
				}
			}
			c.JSON(http.StatusOK, gin.H{"limits": userLimits})
		} else {
			// Return all limits
			c.JSON(http.StatusOK, gin.H{"limits": allLimits})
		}
	}
}

