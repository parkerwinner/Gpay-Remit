package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/yourusername/gpay-remit/config"
	"gorm.io/gorm"
)

type HealthHandler struct {
	DB          *gorm.DB
	Cfg         *config.Config
	RedisClient *redis.Client
}

func NewHealthHandler(db *gorm.DB, cfg *config.Config) *HealthHandler {
	return &HealthHandler{DB: db, Cfg: cfg}
}

func NewHealthHandlerWithRedis(db *gorm.DB, cfg *config.Config, rdb *redis.Client) *HealthHandler {
	return &HealthHandler{DB: db, Cfg: cfg, RedisClient: rdb}
}

type dependencyStatus struct {
	Status  string `json:"status"`
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

type dbPoolMetrics struct {
	MaxOpenConnections int           `json:"max_open_connections"`
	OpenConnections    int           `json:"open_connections"`
	InUse              int           `json:"in_use"`
	Idle               int           `json:"idle"`
	WaitCount          int64         `json:"wait_count"`
	WaitDuration       time.Duration `json:"wait_duration_ns"`
}

type databaseStatus struct {
	dependencyStatus
	Pool *dbPoolMetrics `json:"pool,omitempty"`
}

type healthResponse struct {
	Status       string                 `json:"status"`
	Service      string                 `json:"service"`
	Timestamp    string                 `json:"timestamp"`
	Dependencies map[string]interface{} `json:"dependencies,omitempty"`
}

// checkAll probes every dependency concurrently.
//
// Sequential probing made the endpoint's worst case the *sum* of the individual
// timeouts (2s database + 3s Horizon + 2s Redis = 7s). A Kubernetes probe
// typically gives 1-3s, so a single partitioned dependency would time out the
// probe itself and the pod would be restarted or pulled from service without
// any indication of which dependency was at fault. Run concurrently, the worst
// case is the slowest single check.
func (h *HealthHandler) checkAll() (databaseStatus, dependencyStatus, dependencyStatus) {
	var (
		wg      sync.WaitGroup
		db      databaseStatus
		horizon dependencyStatus
		redis   dependencyStatus
	)

	wg.Add(3)
	go func() { defer wg.Done(); db = h.checkDatabase() }()
	go func() { defer wg.Done(); horizon = h.checkHorizon() }()
	go func() { defer wg.Done(); redis = h.checkRedis() }()
	wg.Wait()

	return db, horizon, redis
}

// Health returns detailed health status including all dependencies.
func (h *HealthHandler) Health(c *gin.Context) {
	dbStatus, horizonStatus, redisStatus := h.checkAll()

	overall := "healthy"
	httpStatus := http.StatusOK
	// "unconfigured" is a known, non-fatal state — Redis is optional.
	if dbStatus.Status != "healthy" || horizonStatus.Status != "healthy" ||
		(redisStatus.Status != "healthy" && redisStatus.Status != "unconfigured") {
		overall = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, healthResponse{
		Status:    overall,
		Service:   "gpay-remit-api",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Dependencies: map[string]interface{}{
			"database": dbStatus,
			"horizon":  horizonStatus,
			"redis":    redisStatus,
		},
	})
}

// Ready checks database, Horizon, and Redis — used for Kubernetes readiness probes.
func (h *HealthHandler) Ready(c *gin.Context) {
	dbStatus, horizonStatus, redisStatus := h.checkAll()

	// "unconfigured" is non-fatal for readiness — allow the pod to serve traffic.
	if dbStatus.Status != "healthy" || horizonStatus.Status != "healthy" ||
		(redisStatus.Status != "healthy" && redisStatus.Status != "unconfigured") {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":   "not_ready",
			"database": dbStatus,
			"horizon":  horizonStatus,
			"redis":    redisStatus,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

// Live reports whether the process itself is running — used for Kubernetes
// liveness probes.
//
// Deliberately does not touch the database, Horizon or Redis. Liveness answers
// "should this container be restarted?", and restarting the API because
// Postgres is briefly unreachable makes an outage worse: every replica dies at
// once and none of them can serve the cached or degraded responses they
// otherwise could. Dependency state belongs on readiness, which pulls the pod
// out of the load balancer without killing it.
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "alive",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *HealthHandler) checkDatabase() databaseStatus {
	if h.DB == nil {
		return databaseStatus{dependencyStatus: dependencyStatus{Status: "unhealthy", Error: "database not configured"}}
	}
	start := time.Now()
	sqlDB, err := h.DB.DB()
	if err != nil {
		return databaseStatus{dependencyStatus: dependencyStatus{Status: "unhealthy", Error: err.Error()}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return databaseStatus{dependencyStatus: dependencyStatus{Status: "unhealthy", Error: err.Error()}}
	}
	pool := collectPoolMetrics(sqlDB)
	return databaseStatus{
		dependencyStatus: dependencyStatus{Status: "healthy", Latency: time.Since(start).String()},
		Pool:             pool,
	}
}

func collectPoolMetrics(sqlDB *sql.DB) *dbPoolMetrics {
	stats := sqlDB.Stats()
	return &dbPoolMetrics{
		MaxOpenConnections: stats.MaxOpenConnections,
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		WaitCount:          stats.WaitCount,
		WaitDuration:       stats.WaitDuration,
	}
}

func (h *HealthHandler) checkHorizon() dependencyStatus {
	if h.Cfg == nil || h.Cfg.HorizonURL == "" {
		return dependencyStatus{Status: "unhealthy", Error: "horizon URL not configured"}
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Cfg.HorizonURL, nil)
	if err != nil {
		return dependencyStatus{Status: "unhealthy", Error: err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return dependencyStatus{Status: "unhealthy", Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return dependencyStatus{Status: "unhealthy", Error: "horizon returned " + resp.Status}
	}
	return dependencyStatus{Status: "healthy", Latency: time.Since(start).String()}
}

func (h *HealthHandler) checkRedis() dependencyStatus {
	if h.RedisClient == nil {
		return dependencyStatus{Status: "unconfigured", Error: "redis client not initialized"}
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := h.RedisClient.Ping(ctx).Err(); err != nil {
		return dependencyStatus{Status: "unhealthy", Error: err.Error()}
	}
	return dependencyStatus{Status: "healthy", Latency: time.Since(start).String()}
}
