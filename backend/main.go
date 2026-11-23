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
	"github.com/Gpay-Remit/gpay-remit/graphql"
	"github.com/Gpay-Remit/gpay-remit/handlers"
	"github.com/Gpay-Remit/gpay-remit/logger"
	"github.com/Gpay-Remit/gpay-remit/metrics"
	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/Gpay-Remit/gpay-remit/workers"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

	readDB := db
	if replicaDB, replicaErr := config.InitReadDB(cfg); replicaErr != nil {
		logger.Log.WithField("error", replicaErr).Warn("Read replica unavailable — using primary for reads")
	} else if replicaDB != nil {
		readDB = replicaDB
		logger.Log.Info("Database read replica connected")
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

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(cors.New(cfg.CORSConfig()))
	router.Use(middleware.PrometheusMetrics())
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.RequestLogger())
	router.Use(middleware.ErrorHandler())
	// Cap request bodies to bound memory use (#381). Routes that need a larger
	// body go in routeBodyLimits, keyed by Gin route pattern, e.g.
	// "/api/v1/uploads": middleware.UploadMaxBodyBytes.
	routeBodyLimits := map[string]int64{}
	router.Use(middleware.BodyLimit(middleware.DefaultMaxBodyBytes, routeBodyLimits))
	router.Use(middleware.VersionMiddleware())
	router.Use(middleware.TLSMiddleware())
	router.Use(middleware.CSRFProtection())
	router.Use(middleware.SecurityHeadersMiddleware())

	registerRoutes(router, db, readDB, cfg)

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	baseCtx, cancelWorkers := context.WithCancel(context.Background())
	utils.StartRedisStatsCollector(baseCtx, 30*time.Second)
	var wg sync.WaitGroup
	middleware.StartIdempotencyCleanupScheduler(baseCtx, &wg, db, time.Hour)
	workers.StartMonitor(baseCtx, &wg)
	workers.StartWebhookRetryWorker(baseCtx, &wg, db)
	workers.StartPaymentRequestExpiryWorker(baseCtx, &wg, db)
	if cfg.RecurringWorkerEnabled {
		workers.StartRecurringPaymentWorker(baseCtx, &wg, db, cfg)
	}

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
	if readDB != db {
		if sqlDB, err := readDB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				logger.Log.WithField("error", err).Warn("Failed to close read replica")
			}
		}
	}
}

// registerRoutes mounts every HTTP route on router. It is separate from
// main so tests can enumerate the real route table (see
// openapi_routes_test.go) without a database: handler constructors only
// store their dependencies, so nil db is fine for registration.
func registerRoutes(router *gin.Engine, db, readDB *gorm.DB, cfg *config.Config) {
	router.GET("/metrics", gin.WrapH(metrics.Handler()))

	healthHandler := handlers.NewHealthHandlerWithRedis(db, cfg, utils.RedisClient)
	router.GET("/health", healthHandler.Health)
	router.GET("/health/ready", healthHandler.Ready)
	router.GET("/health/live", healthHandler.Live)

	router.GET("/api/docs", handlers.DocsUI)
	router.GET("/api/docs/openapi.yaml", handlers.DocsSpec)

	gqlServer := graphql.NewServer(db, cfg)
	router.GET("/playground", gqlServer.PlaygroundHandler())
	router.GET("/graphql/playground", gqlServer.PlaygroundHandler())
	router.POST("/graphql", gqlServer.QueryHandler())
	router.GET("/graphql", gqlServer.PlaygroundHandler())

	api := router.Group("/api/v1")
	{
		emailService := services.NewEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom, cfg.EmailEnabled)
		authHandler := handlers.NewAuthHandler(db, cfg, emailService)
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
		api.POST("/auth/refresh", authHandler.Refresh)
		api.POST("/auth/forgot-password", authHandler.ForgotPassword)
		api.POST("/auth/reset-password", authHandler.ResetPassword)
		api.POST("/auth/request-unlock", authHandler.RequestUnlock)
		api.POST("/auth/unlock", authHandler.UnlockAccount)

		api.POST("/users", authHandler.Register)

		protected := api.Group("/")
		protected.Use(middleware.JwtAuthMiddleware(db, cfg))
		protected.Use(middleware.AuditTrail(db))
		{
			protected.POST("/auth/logout", authHandler.Logout)
			protected.POST("/auth/change-password", authHandler.ChangePassword)
			// 2FA endpoints — pquerna/otp based TOTP
			protected.POST("/auth/mfa/setup", authHandler.SetupMFA)
			protected.POST("/auth/mfa/verify", authHandler.VerifyMFA)
			protected.POST("/auth/mfa/disable", authHandler.DisableMFA)
			protected.GET("/auth/mfa/status", func(c *gin.Context) {
				userID, _ := c.Get("userID")
				var user models.User
				if err := db.First(&user, userID).Error; err != nil {
					c.JSON(404, gin.H{"error": "User not found"})
					return
				}
				c.JSON(200, gin.H{"mfa_enabled": user.MFAEnabled})
			})
			remittanceHandler := handlers.NewRemittanceHandler(db, cfg, readDB)
			searchHandler := handlers.NewSearchHandler(db, cfg, readDB)
			protected.GET("/search/transactions", searchHandler.SearchTransactions)
			protected.POST("/remittances/create", remittanceHandler.CreateRemittance)
			protected.POST("/remittances/batch", remittanceHandler.CreateBatchRemittance)
			protected.POST("/remittances", remittanceHandler.SendRemittance)
			protected.GET("/remittances/:id", remittanceHandler.GetRemittance)
			protected.GET("/remittances", remittanceHandler.ListRemittances)
			protected.POST("/remittances/:id/complete", middleware.RequireRole("admin"), remittanceHandler.CompleteRemittance)

			templateHandler := handlers.NewPaymentTemplateHandler(db, cfg, utils.NewStellarClient(cfg.StellarHorizonURL, cfg.NetworkPassphrase))
			protected.POST("/templates", templateHandler.CreateTemplate)
			protected.GET("/templates", templateHandler.ListTemplates)
			protected.GET("/templates/:id", templateHandler.GetTemplate)
			protected.PUT("/templates/:id", templateHandler.UpdateTemplate)
			protected.DELETE("/templates/:id", templateHandler.DeleteTemplate)
			protected.POST("/templates/:id/send", templateHandler.SendFromTemplate)

			recurringHandler := handlers.NewRecurringScheduleHandler(db, cfg, utils.NewStellarClient(cfg.StellarHorizonURL, cfg.NetworkPassphrase))
			protected.POST("/recurring-schedules", recurringHandler.CreateSchedule)
			protected.GET("/recurring-schedules", recurringHandler.ListSchedules)
			protected.GET("/recurring-schedules/:id", recurringHandler.GetSchedule)
			protected.POST("/recurring-schedules/:id/pause", recurringHandler.PauseSchedule)
			protected.POST("/recurring-schedules/:id/resume", recurringHandler.ResumeSchedule)
			protected.POST("/recurring-schedules/:id/cancel", recurringHandler.CancelSchedule)

			walletHandler := handlers.NewWalletHandler(db, cfg, utils.NewStellarClient(cfg.StellarHorizonURL, cfg.NetworkPassphrase))
			protected.GET("/wallet/balances", walletHandler.GetBalances)
			protected.GET("/wallet/preferences", walletHandler.GetPreferences)
			protected.PUT("/wallet/preferences", walletHandler.UpdatePreferences)

			protected.POST("/invoices", remittanceHandler.CreateInvoice)
			protected.GET("/invoices", remittanceHandler.ListInvoices)
			protected.GET("/invoices/:id", remittanceHandler.GetInvoice)

			contactHandler := handlers.NewContactHandler(db, cfg, utils.NewStellarClient(cfg.StellarHorizonURL, cfg.NetworkPassphrase))
			protected.POST("/contacts", contactHandler.CreateContact)
			protected.GET("/contacts", contactHandler.ListContacts)
			protected.GET("/contacts/:id", contactHandler.GetContact)
			protected.PUT("/contacts/:id", contactHandler.UpdateContact)
			protected.DELETE("/contacts/:id", contactHandler.DeleteContact)
			protected.POST("/contacts/:id/verify", contactHandler.VerifyContact)

			feeService := services.NewFeeService(cfg)
			feeHandler := handlers.NewFeeHandler(feeService)
			protected.GET("/fees/calculate", feeHandler.Calculate)

			exchangeRateHandler := handlers.NewExchangeRateHandler(cfg)
			protected.GET("/exchange-rates", middleware.RateLimitMiddleware(cfg), exchangeRateHandler.GetRate)

			auditHandler := handlers.NewAuditLogHandler(db)
			protected.GET("/audit/logs", middleware.RequireRole("admin"), auditHandler.List)

			exportHandler := handlers.NewExportHandler(db, readDB)
			protected.GET("/transactions/export", exportHandler.ExportTransactions)

			// Admin rate limit management endpoints
			protected.POST("/admin/rate-limit/reset", middleware.RequireRole("admin"), middleware.AdminResetRateLimit(cfg))
			protected.GET("/admin/rate-limit/view", middleware.RequireRole("admin"), middleware.AdminViewRateLimits(cfg))

			// Payment request endpoints
			emailService := services.NewEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom, cfg.EmailEnabled)
			paymentRequestHandler := handlers.NewPaymentRequestHandler(db, feeService, emailService)
			protected.POST("/payment-requests", paymentRequestHandler.CreatePaymentRequest)
			protected.GET("/payment-requests", paymentRequestHandler.ListPaymentRequests)
			protected.GET("/payment-requests/:id", paymentRequestHandler.GetPaymentRequest)
			protected.POST("/payment-requests/:id/accept", paymentRequestHandler.AcceptPaymentRequest)
			protected.POST("/payment-requests/:id/reject", paymentRequestHandler.RejectPaymentRequest)
			protected.POST("/payment-requests/:id/cancel", paymentRequestHandler.CancelPaymentRequest)

			// Webhook endpoints
			webhookHandler := handlers.NewWebhookHandler(db)
			protected.POST("/webhooks", webhookHandler.CreateWebhook)
			protected.GET("/webhooks", webhookHandler.ListWebhooks)
			protected.GET("/webhooks/:id", webhookHandler.GetWebhook)
			protected.PUT("/webhooks/:id", webhookHandler.UpdateWebhook)
			protected.DELETE("/webhooks/:id", webhookHandler.DeleteWebhook)
			protected.GET("/webhooks/:id/deliveries", webhookHandler.GetWebhookDeliveries)
			protected.POST("/webhooks/deliveries/:delivery_id/retry", webhookHandler.RetryWebhookDelivery)

			analyticsHandler := handlers.NewAnalyticsHandler(db, readDB)
			protected.GET("/analytics/volume", middleware.RequireRole("admin"), analyticsHandler.GetVolumeMetrics)
			protected.GET("/analytics/fees", middleware.RequireRole("admin"), analyticsHandler.GetFeeMetrics)
			protected.GET("/analytics/success-rate", middleware.RequireRole("admin"), analyticsHandler.GetSuccessRate)
			protected.GET("/analytics/top-corridors", middleware.RequireRole("admin"), analyticsHandler.GetTopCorridors)
			protected.GET("/analytics/sla", middleware.RequireRole("admin"), analyticsHandler.GetSLAMetrics)

		}
	}

	api2 := router.Group("/api/v2")
	api2.Use(middleware.RequireVersion("v2"))
	{
		emailService := services.NewEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom, cfg.EmailEnabled)
		authHandler := handlers.NewAuthHandler(db, cfg, emailService)
		api2.POST("/auth/register", authHandler.Register)
		api2.POST("/auth/login", authHandler.Login)
		api2.POST("/auth/refresh", authHandler.Refresh)
		api2.POST("/auth/forgot-password", authHandler.ForgotPassword)
		api2.POST("/auth/reset-password", authHandler.ResetPassword)
		api2.POST("/auth/request-unlock", authHandler.RequestUnlock)
		api2.POST("/auth/unlock", authHandler.UnlockAccount)

		api2.POST("/users", authHandler.Register)

		protected := api2.Group("/")
		protected.Use(middleware.JwtAuthMiddleware(db, cfg))
		protected.Use(middleware.AuditTrail(db))
		{
			protected.POST("/auth/logout", authHandler.Logout)
			protected.POST("/auth/change-password", authHandler.ChangePassword)
			protected.POST("/auth/mfa/setup", authHandler.SetupMFA)
			protected.POST("/auth/mfa/verify", authHandler.VerifyMFA)
			protected.POST("/auth/mfa/disable", authHandler.DisableMFA)
			protected.GET("/auth/mfa/status", func(c *gin.Context) {
				userID, _ := c.Get("userID")
				var user models.User
				if err := db.First(&user, userID).Error; err != nil {
					c.JSON(404, gin.H{"error": "User not found"})
					return
				}
				c.JSON(200, gin.H{"mfa_enabled": user.MFAEnabled})
			})
			remittanceHandler := handlers.NewRemittanceHandler(db, cfg, readDB)
			searchHandler := handlers.NewSearchHandler(db, cfg, readDB)
			protected.GET("/search/transactions", searchHandler.SearchTransactions)
			protected.POST("/remittances/create", remittanceHandler.CreateRemittance)
			protected.POST("/remittances/batch", remittanceHandler.CreateBatchRemittance)
			protected.POST("/remittances", remittanceHandler.SendRemittance)
			protected.GET("/remittances/:id", remittanceHandler.GetRemittance)
			protected.GET("/remittances", remittanceHandler.ListRemittances)
			protected.POST("/remittances/:id/complete", middleware.RequireRole("admin"), remittanceHandler.CompleteRemittance)

			templateHandler := handlers.NewPaymentTemplateHandler(db, cfg, utils.NewStellarClient(cfg.StellarHorizonURL, cfg.NetworkPassphrase))
			protected.POST("/templates", templateHandler.CreateTemplate)
			protected.GET("/templates", templateHandler.ListTemplates)
			protected.GET("/templates/:id", templateHandler.GetTemplate)
			protected.PUT("/templates/:id", templateHandler.UpdateTemplate)
			protected.DELETE("/templates/:id", templateHandler.DeleteTemplate)
			protected.POST("/templates/:id/send", templateHandler.SendFromTemplate)

			recurringHandler := handlers.NewRecurringScheduleHandler(db, cfg, utils.NewStellarClient(cfg.StellarHorizonURL, cfg.NetworkPassphrase))
			protected.POST("/recurring-schedules", recurringHandler.CreateSchedule)
			protected.GET("/recurring-schedules", recurringHandler.ListSchedules)
			protected.GET("/recurring-schedules/:id", recurringHandler.GetSchedule)
			protected.POST("/recurring-schedules/:id/pause", recurringHandler.PauseSchedule)
			protected.POST("/recurring-schedules/:id/resume", recurringHandler.ResumeSchedule)
			protected.POST("/recurring-schedules/:id/cancel", recurringHandler.CancelSchedule)

			walletHandler := handlers.NewWalletHandler(db, cfg, utils.NewStellarClient(cfg.StellarHorizonURL, cfg.NetworkPassphrase))
			protected.GET("/wallet/balances", walletHandler.GetBalances)
			protected.GET("/wallet/preferences", walletHandler.GetPreferences)
			protected.PUT("/wallet/preferences", walletHandler.UpdatePreferences)

			protected.POST("/invoices", remittanceHandler.CreateInvoice)
			protected.GET("/invoices", remittanceHandler.ListInvoices)
			protected.GET("/invoices/:id", remittanceHandler.GetInvoice)

			contactHandler := handlers.NewContactHandler(db, cfg, utils.NewStellarClient(cfg.StellarHorizonURL, cfg.NetworkPassphrase))
			protected.POST("/contacts", contactHandler.CreateContact)
			protected.GET("/contacts", contactHandler.ListContacts)
			protected.GET("/contacts/:id", contactHandler.GetContact)
			protected.PUT("/contacts/:id", contactHandler.UpdateContact)
			protected.DELETE("/contacts/:id", contactHandler.DeleteContact)
			protected.POST("/contacts/:id/verify", contactHandler.VerifyContact)

			feeService := services.NewFeeService(cfg)
			feeHandler := handlers.NewFeeHandler(feeService)
			protected.GET("/fees/calculate", feeHandler.Calculate)

			exchangeRateHandler := handlers.NewExchangeRateHandler(cfg)
			protected.GET("/exchange-rates", middleware.RateLimitMiddleware(cfg), exchangeRateHandler.GetRate)

			auditHandler := handlers.NewAuditLogHandler(db)
			protected.GET("/audit/logs", middleware.RequireRole("admin"), auditHandler.List)

			exportHandler := handlers.NewExportHandler(db, readDB)
			protected.GET("/transactions/export", exportHandler.ExportTransactions)

			protected.POST("/admin/rate-limit/reset", middleware.RequireRole("admin"), middleware.AdminResetRateLimit(cfg))
			protected.GET("/admin/rate-limit/view", middleware.RequireRole("admin"), middleware.AdminViewRateLimits(cfg))

			// Payment request endpoints
			emailService := services.NewEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom, cfg.EmailEnabled)
			paymentRequestHandler := handlers.NewPaymentRequestHandler(db, feeService, emailService)
			protected.POST("/payment-requests", paymentRequestHandler.CreatePaymentRequest)
			protected.GET("/payment-requests", paymentRequestHandler.ListPaymentRequests)
			protected.GET("/payment-requests/:id", paymentRequestHandler.GetPaymentRequest)
			protected.POST("/payment-requests/:id/accept", paymentRequestHandler.AcceptPaymentRequest)
			protected.POST("/payment-requests/:id/reject", paymentRequestHandler.RejectPaymentRequest)
			protected.POST("/payment-requests/:id/cancel", paymentRequestHandler.CancelPaymentRequest)

			webhookHandler := handlers.NewWebhookHandler(db)
			protected.POST("/webhooks", webhookHandler.CreateWebhook)
			protected.GET("/webhooks", webhookHandler.ListWebhooks)
			protected.GET("/webhooks/:id", webhookHandler.GetWebhook)
			protected.PUT("/webhooks/:id", webhookHandler.UpdateWebhook)
			protected.DELETE("/webhooks/:id", webhookHandler.DeleteWebhook)
			protected.GET("/webhooks/:id/deliveries", webhookHandler.GetWebhookDeliveries)
			protected.POST("/webhooks/deliveries/:delivery_id/retry", webhookHandler.RetryWebhookDelivery)

			analyticsHandler := handlers.NewAnalyticsHandler(db, readDB)
			protected.GET("/analytics/volume", middleware.RequireRole("admin"), analyticsHandler.GetVolumeMetrics)
			protected.GET("/analytics/fees", middleware.RequireRole("admin"), analyticsHandler.GetFeeMetrics)
			protected.GET("/analytics/success-rate", middleware.RequireRole("admin"), analyticsHandler.GetSuccessRate)
			protected.GET("/analytics/top-corridors", middleware.RequireRole("admin"), analyticsHandler.GetTopCorridors)
			protected.GET("/analytics/sla", middleware.RequireRole("admin"), analyticsHandler.GetSLAMetrics)

		}
	}
}
