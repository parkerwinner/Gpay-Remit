package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/config"
)

func setupHealthRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	cfg := &config.Config{HorizonURL: "https://horizon-testnet.stellar.org"}
	// No Redis client — tests run without a real Redis; checkRedis returns "unconfigured".
	handler := NewHealthHandler(db, cfg)

	router := gin.New()
	router.GET("/health", handler.Health)
	router.GET("/health/ready", handler.Ready)
	router.GET("/health/live", handler.Live)
	return router
}

func TestHealthLive(t *testing.T) {
	router := setupHealthRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health/live", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "alive", resp["status"])
	assert.NotEmpty(t, resp["timestamp"])
}

func TestHealthNilDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHealthHandler(nil, &config.Config{HorizonURL: ""})
	router := gin.New()
	router.GET("/health", handler.Health)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(w, req)

	// Nil DB and empty Horizon URL both unhealthy → 503
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "degraded", resp["status"])
}

func TestHealthReadyNilDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHealthHandler(nil, &config.Config{HorizonURL: ""})
	router := gin.New()
	router.GET("/health/ready", handler.Ready)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health/ready", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHealthResponseFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Use in-memory SQLite DB; Horizon will likely succeed or timeout
	db := setupTestDB()
	handler := NewHealthHandler(db, &config.Config{HorizonURL: "https://horizon-testnet.stellar.org"})
	router := gin.New()
	router.GET("/health", handler.Health)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp["status"])
	assert.Equal(t, "gpay-remit-api", resp["service"])
	assert.NotEmpty(t, resp["timestamp"])
	assert.NotNil(t, resp["dependencies"])
}

// TestHealthRedisUnconfigured verifies that a nil RedisClient reports
// "unconfigured" rather than "unhealthy" and does not panic.
func TestHealthRedisUnconfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// NewHealthHandler does not inject a Redis client → RedisClient is nil.
	handler := NewHealthHandler(nil, &config.Config{HorizonURL: ""})

	status := handler.checkRedis()

	assert.Equal(t, "unconfigured", status.Status)
	assert.NotEmpty(t, status.Error)
}

// TestHealthWithRedisNilDoesNotPanic exercises the full /health endpoint
// when no Redis client is wired in, ensuring the handler degrades gracefully.
func TestHealthWithRedisNilDoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHealthHandler(nil, &config.Config{HorizonURL: ""})
	router := gin.New()
	router.GET("/health", handler.Health)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	assert.NotPanics(t, func() { router.ServeHTTP(w, req) })

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	deps, _ := resp["dependencies"].(map[string]interface{})
	redis, _ := deps["redis"].(map[string]interface{})
	assert.Equal(t, "unconfigured", redis["status"])
}
