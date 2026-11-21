package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/logger"
	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/Gpay-Remit/gpay-remit/workers"
)

// shuttingDown signals goroutines to stop when a shutdown signal is received
var shuttingDown atomic.Bool

func main() {
	env := os.Getenv("APP_ENV")
	logger.Init(env)

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Log.WithField("error", err).Fatal("Failed to load config")
	}

	db, err := config.InitDB(cfg)
	if err != nil {
		logger.Log.WithField("error", err).Fatal("Failed to connect to database")
	}

	if err := models.EncryptLegacyWebhookSecrets(db); err != nil {
		logger.Log.WithField("error", err).Fatal("Failed to encrypt legacy webhook secrets")
	}

	// Initialize Redis — non-fatal: the app runs without Redis but cache and
	// some health-check fields will be degraded.
	if err := utils.InitRedis(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB); err != nil {
		logger.Log.WithField("error", err).Warn("Redis unavailable — continuing without cache")
	} else {
		logger.Log.WithField("addr", cfg.RedisAddr).Info("Redis connected")
	}

	router := setupRouter(cfg, db)

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	baseCtx, cancelWorkers := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	middleware.StartIdempotencyCleanupScheduler(baseCtx, &wg, db, time.Hour)
	workers.StartMonitor(baseCtx, &wg)
	workers.StartWebhookRetryWorker(baseCtx, &wg, db)
	workers.StartPaymentRequestExpiryWorker(baseCtx, &wg, db)

	errCh := make(chan error, 1)
	go func() {
		logger.Log.WithField("port", cfg.Port).Info("Starting Gpay-Remit API server")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		shuttingDown.Store(true)
		logger.Log.WithField("signal", sig.String()).Info("Shutdown signal received")
	case err := <-errCh:
		logger.Log.WithField("error", err).Fatal("Server failed unexpectedly")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Log.Info("Stopping background workers...")
	cancelWorkers()

	logger.Log.Info("Shutting down HTTP server...")
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Log.WithField("error", err).Error("HTTP server shutdown error")
	} else {
		logger.Log.Info("HTTP server stopped accepting new requests")
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Log.Info("Background workers stopped")
	case <-shutdownCtx.Done():
		logger.Log.Warn("Timeout waiting for background workers to stop")
	}

	logger.Log.Info("Stopping database connection pool monitor...")
	config.StopConnectionPoolMonitor()

	logger.Log.Info("Closing database connection...")
	if sqlDB, err := db.DB(); err != nil {
		logger.Log.WithField("error", err).Error("Failed to get sql.DB for closing")
	} else if err := sqlDB.Close(); err != nil {
		logger.Log.WithField("error", err).Error("Failed to close database connection")
	} else {
		logger.Log.Info("Database connection closed")
	}
}
