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
