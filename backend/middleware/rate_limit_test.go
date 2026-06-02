package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/config"
)

func TestRateLimiter_IncrementAndCheck(t *testing.T) {
	cfg := &config.Config{}
	limiter := GetRateLimiter(cfg)
	
	key := "test:user:1"
	maxRequests := 5
	window := time.Minute
	
	// First 5 requests should be allowed
	for i := 0; i < maxRequests; i++ {
		allowed, remaining, _ := limiter.IncrementAndCheck(key, maxRequests, window)
		assert.True(t, allowed, fmt.Sprintf("Request %d should be allowed", i+1))
		assert.Equal(t, maxRequests-i-1, remaining)
	}
	
	// 6th request should be denied
	allowed, remaining, _ := limiter.IncrementAndCheck(key, maxRequests, window)
	assert.False(t, allowed, "Request should be denied after limit")
	assert.Equal(t, 0, remaining)
}

func TestRateLimiter_WindowReset(t *testing.T) {
	cfg := &config.Config{}
	limiter := GetRateLimiter(cfg)
	
	key := "test:user:2"
	maxRequests := 3
	window := 100 * time.Millisecond
	
	// Use up the limit
	for i := 0; i < maxRequests; i++ {
		allowed, _, _ := limiter.IncrementAndCheck(key, maxRequests, window)
		assert.True(t, allowed)
	}
	
	// Should be denied
	allowed, _, _ := limiter.IncrementAndCheck(key, maxRequests, window)
	assert.False(t, allowed)
	
	// Wait for window to reset
	time.Sleep(150 * time.Millisecond)
	
	// Should be allowed again
	allowed, remaining, _ := limiter.IncrementAndCheck(key, maxRequests, window)
	assert.True(t, allowed)
	assert.Equal(t, maxRequests-1, remaining)
}

func TestRateLimiter_ResetUserLimit(t *testing.T) {
	cfg := &config.Config{}
	limiter := GetRateLimiter(cfg)
	
	key := "test:user:3"
	maxRequests := 2
	window := time.Minute
	
	// Use up the limit
	for i := 0; i < maxRequests; i++ {
		limiter.IncrementAndCheck(key, maxRequests, window)
	}
	
	// Should be denied
	allowed, _, _ := limiter.IncrementAndCheck(key, maxRequests, window)
	assert.False(t, allowed)
	
	// Reset the limit
	limiter.ResetUserLimit(key)
	
	// Should be allowed again
	allowed, _, _ = limiter.IncrementAndCheck(key, maxRequests, window)
	assert.True(t, allowed)
}

func TestRateLimitMiddleware_WithUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	
	router := gin.New()
	router.Use(RateLimitMiddleware(cfg))
	router.POST("/api/v1/remittances", func(c *gin.Context) {
		c.Set("userID", uint(123))
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})
	
	// Make requests up to the limit (10 for remittances endpoint)
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/remittances", nil)
		
		// Simulate authenticated user
		router.Use(func(c *gin.Context) {
			c.Set("userID", uint(123))
			c.Next()
		})
		
		router.ServeHTTP(w, req)
		
		if i < 10 {
			assert.Equal(t, http.StatusOK, w.Code, fmt.Sprintf("Request %d should succeed", i+1))
			assert.NotEmpty(t, w.Header().Get("X-RateLimit-Limit"))
			assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
			assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
		}
	}
}

func TestRateLimitMiddleware_ExceedLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	limiter := GetRateLimiter(cfg)
	
	// Clear any existing limits
	limiter.ResetUserLimit("user:456:endpoint:POST /api/v1/auth/login")
	
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(456))
		c.Next()
	})
	router.Use(RateLimitMiddleware(cfg))
	router.POST("/api/v1/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})
	
	// Make requests up to limit (5 for login endpoint)
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/auth/login", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
	
	// Next request should be rate limited
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/login", nil)
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

func TestRateLimitMiddleware_NoUser_UsesIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	
	router := gin.New()
	router.Use(RateLimitMiddleware(cfg))
	router.GET("/api/v1/public", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})
	
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/public", nil)
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Limit"))
}

func TestAdminResetRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	limiter := GetRateLimiter(cfg)
	
	// Set up some limits
	limiter.IncrementAndCheck("user:789:endpoint:POST /api/v1/remittances", 10, time.Minute)
	limiter.IncrementAndCheck("user:789:endpoint:GET /api/v1/remittances", 10, time.Minute)
	
	router := gin.New()
	router.POST("/admin/rate-limit/reset", AdminResetRateLimit(cfg))
	
	// Test reset specific endpoint
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/rate-limit/reset?user_id=789&endpoint=POST /api/v1/remittances", nil)
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Rate limit reset")
}

func TestAdminResetRateLimit_AllEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	limiter := GetRateLimiter(cfg)
	
	// Set up multiple limits for same user
	limiter.IncrementAndCheck("user:999:endpoint:POST /api/v1/remittances", 10, time.Minute)
	limiter.IncrementAndCheck("user:999:endpoint:GET /api/v1/remittances", 10, time.Minute)
	limiter.IncrementAndCheck("user:999:endpoint:POST /api/v1/invoices", 10, time.Minute)
	
	router := gin.New()
	router.POST("/admin/rate-limit/reset", AdminResetRateLimit(cfg))
	
	// Test reset all endpoints for user
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/rate-limit/reset?user_id=999", nil)
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "rate limits for user 999")
}

func TestAdminViewRateLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	limiter := GetRateLimiter(cfg)
	
	// Set up some limits
	limiter.IncrementAndCheck("user:111:endpoint:POST /api/v1/remittances", 10, time.Minute)
	
	router := gin.New()
	router.GET("/admin/rate-limit/view", AdminViewRateLimits(cfg))
	
	// Test view all limits
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/rate-limit/view", nil)
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "limits")
}

func TestAdminViewRateLimits_FilterByUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	limiter := GetRateLimiter(cfg)
	
	// Set up limits for different users
	limiter.IncrementAndCheck("user:222:endpoint:POST /api/v1/remittances", 10, time.Minute)
	limiter.IncrementAndCheck("user:333:endpoint:POST /api/v1/remittances", 10, time.Minute)
	
	router := gin.New()
	router.GET("/admin/rate-limit/view", AdminViewRateLimits(cfg))
	
	// Test view limits for specific user
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/rate-limit/view?user_id=222", nil)
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "user:222")
}

func TestGetAllLimits(t *testing.T) {
	cfg := &config.Config{}
	limiter := GetRateLimiter(cfg)
	
	// Add some limits
	limiter.IncrementAndCheck("user:test1:endpoint:GET /test", 10, time.Minute)
	limiter.IncrementAndCheck("user:test2:endpoint:POST /test", 10, time.Minute)
	
	limits := limiter.GetAllLimits()
	assert.NotEmpty(t, limits)
	
	// Verify it's a copy (modifying it shouldn't affect the original)
	for key := range limits {
		limits[key].Count = 999
	}
	
	originalLimit := limiter.GetLimit("user:test1:endpoint:GET /test")
	assert.NotEqual(t, 999, originalLimit.Count)
}
