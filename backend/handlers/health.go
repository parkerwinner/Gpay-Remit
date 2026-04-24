package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/config"
	"gorm.io/gorm"
)

type HealthHandler struct {
	DB  *gorm.DB
	Cfg *config.Config
}

func NewHealthHandler(db *gorm.DB, cfg *config.Config) *HealthHandler {
	return &HealthHandler{DB: db, Cfg: cfg}
}

type dependencyStatus struct {
	Status  string `json:"status"`
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

type healthResponse struct {
	Status       string                      `json:"status"`
	Service      string                      `json:"service"`
	Timestamp    string                      `json:"timestamp"`
	Dependencies map[string]dependencyStatus `json:"dependencies,omitempty"`
}

// Health returns detailed health status including all dependencies.
func (h *HealthHandler) Health(c *gin.Context) {
	dbStatus := h.checkDatabase()
	horizonStatus := h.checkHorizon()

	overall := "healthy"
	httpStatus := http.StatusOK
	if dbStatus.Status != "healthy" || horizonStatus.Status != "healthy" {
		overall = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, healthResponse{
		Status:    overall,
		Service:   "gpay-remit-api",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Dependencies: map[string]dependencyStatus{
			"database": dbStatus,
			"horizon":  horizonStatus,
		},
	})
}

// Ready checks database and Horizon — used for Kubernetes readiness probes.
func (h *HealthHandler) Ready(c *gin.Context) {
	dbStatus := h.checkDatabase()
	horizonStatus := h.checkHorizon()

	if dbStatus.Status != "healthy" || horizonStatus.Status != "healthy" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":   "not_ready",
			"database": dbStatus,
			"horizon":  horizonStatus,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

// Live checks only critical in-process state — used for Kubernetes liveness probes.
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "alive",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *HealthHandler) checkDatabase() dependencyStatus {
	if h.DB == nil {
		return dependencyStatus{Status: "unhealthy", Error: "database not configured"}
	}
	start := time.Now()
	sqlDB, err := h.DB.DB()
	if err != nil {
		return dependencyStatus{Status: "unhealthy", Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return dependencyStatus{Status: "unhealthy", Error: err.Error()}
	}
	return dependencyStatus{Status: "healthy", Latency: time.Since(start).String()}
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
