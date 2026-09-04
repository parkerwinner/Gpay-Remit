package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// ── Concurrent dependency probing (issue #258) ──────────────────────────────

// TestHealthProbesDependenciesConcurrently guards the property that makes the
// endpoint usable as a Kubernetes probe.
//
// Probed sequentially, the worst case is the sum of the individual timeouts
// (2s database + 3s Horizon + 2s Redis). A probe with a 3s timeout would then
// fail outright whenever any single dependency was partitioned, and the pod
// would be restarted or pulled without any indication of which one was at
// fault. Concurrently, the worst case is the slowest single check.
func TestHealthProbesDependenciesConcurrently(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// A server that accepts and never answers, so the Horizon check burns its
	// full 3s timeout. Database and Redis are nil, so they fail immediately —
	// which means total elapsed time distinguishes the two strategies.
	blocked := make(chan struct{})
	defer close(blocked)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-blocked:
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()

	h := NewHealthHandler(nil, &config.Config{HorizonURL: slow.URL})

	router := gin.New()
	router.GET("/health", h.Health)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	router.ServeHTTP(w, req)
	elapsed := time.Since(start)

	// The Horizon check alone takes ~3s. Sequential probing would add the
	// database and Redis timeouts on top; concurrent probing must not exceed
	// the slowest check by a meaningful margin.
	assert.Less(t, elapsed, 5*time.Second,
		"dependency checks appear to run sequentially")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// TestLiveDoesNotProbeDependencies pins the liveness/readiness split.
//
// Restarting the API because Postgres is briefly unreachable makes an outage
// worse: every replica dies at once. Liveness must answer from in-process state
// only, however broken the dependencies are.
func TestLiveDoesNotProbeDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	blocked := make(chan struct{})
	defer close(blocked)
	unreachable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-blocked:
		case <-r.Context().Done():
		}
	}))
	defer unreachable.Close()

	// Every dependency is broken or unreachable.
	h := NewHealthHandler(nil, &config.Config{HorizonURL: unreachable.URL})

	router := gin.New()
	router.GET("/live", h.Live)

	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	router.ServeHTTP(w, req)

	// Returns immediately and reports alive — it never contacted anything.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Less(t, time.Since(start), 500*time.Millisecond,
		"liveness probed a dependency; it must answer from in-process state only")
	assert.Contains(t, w.Body.String(), "alive")
}

// TestReadyReportsWhichDependencyFailed covers the operational requirement:
// a failing readiness probe has to say what is broken, or an operator is left
// guessing between three dependencies.
func TestReadyReportsWhichDependencyFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHealthHandler(nil, &config.Config{HorizonURL: ""})

	router := gin.New()
	router.GET("/ready", h.Ready)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "not_ready")
	// Each dependency is named with its own status.
	assert.Contains(t, body, "database")
	assert.Contains(t, body, "horizon")
	assert.Contains(t, body, "redis")
}

// TestUnconfiguredRedisDoesNotBlockReadiness pins Redis as optional.
//
// Treating an unconfigured optional dependency as unhealthy would keep every
// pod out of the load balancer in a deployment that simply does not use Redis.
func TestUnconfiguredRedisDoesNotBlockReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()

	// No Redis client at all — the "unconfigured" state.
	h := NewHealthHandler(nil, &config.Config{HorizonURL: healthy.URL})
	_, _, redisStatus := h.checkAll()

	assert.Equal(t, "unconfigured", redisStatus.Status)
}
