package main

import (
	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/graphql"
	"github.com/Gpay-Remit/gpay-remit/handlers"
	"github.com/Gpay-Remit/gpay-remit/metrics"
	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupRouter builds the HTTP router with every middleware and route the API
// serves. It is separate from main so tests can enumerate the registered
// routes (see openapi_routes_test.go) without starting the server, the
// background workers or a real database connection: registering a route
// never calls its handler.
func setupRouter(cfg *config.Config, db *gorm.DB) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(cors.New(cfg.CORSConfig()))
	router.Use(middleware.PrometheusMetrics())
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.RequestLogger())
	router.Use(middleware.ErrorHandler())
	router.Use(middleware.VersionMiddleware())
	router.Use(middleware.TLSMiddleware())
	router.Use(middleware.CSRFProtection())

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
			remittanceHandler := handlers.NewRemittanceHandler(db, cfg)
			protected.POST("/remittances/create", remittanceHandler.CreateRemittance)
			protected.POST("/remittances", remittanceHandler.SendRemittance)
			protected.GET("/remittances/:id", remittanceHandler.GetRemittance)
			protected.GET("/remittances", remittanceHandler.ListRemittances)
			protected.POST("/remittances/:id/complete", middleware.RequireRole("admin"), remittanceHandler.CompleteRemittance)

			protected.POST("/invoices", remittanceHandler.CreateInvoice)
			protected.GET("/invoices", remittanceHandler.ListInvoices)
			protected.GET("/invoices/:id", remittanceHandler.GetInvoice)

			feeService := services.NewFeeService(cfg)
			feeHandler := handlers.NewFeeHandler(feeService)
			protected.GET("/fees/calculate", feeHandler.Calculate)

			exchangeRateHandler := handlers.NewExchangeRateHandler(cfg)
			protected.GET("/exchange-rates", middleware.RateLimitMiddleware(cfg), exchangeRateHandler.GetRate)

			auditHandler := handlers.NewAuditLogHandler(db)
			protected.GET("/audit/logs", middleware.RequireRole("admin"), auditHandler.List)

			exportHandler := handlers.NewExportHandler(db)
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

			// Saved recipients. The handler, its tests and the OpenAPI spec
			// existed, but the routes were never registered (#379).
			contactHandler := handlers.NewContactHandler(db, cfg, utils.NewStellarClient(cfg.HorizonURL, cfg.NetworkPassphrase))
			protected.POST("/contacts", contactHandler.CreateContact)
			protected.GET("/contacts", contactHandler.ListContacts)
			protected.GET("/contacts/:id", contactHandler.GetContact)
			protected.PUT("/contacts/:id", contactHandler.UpdateContact)
			protected.DELETE("/contacts/:id", contactHandler.DeleteContact)
			protected.POST("/contacts/:id/verify", contactHandler.VerifyContact)

			analyticsHandler := handlers.NewAnalyticsHandler(db)
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
			remittanceHandler := handlers.NewRemittanceHandler(db, cfg)
			protected.POST("/remittances/create", remittanceHandler.CreateRemittance)
			protected.POST("/remittances", remittanceHandler.SendRemittance)
			protected.GET("/remittances/:id", remittanceHandler.GetRemittance)
			protected.GET("/remittances", remittanceHandler.ListRemittances)
			protected.POST("/remittances/:id/complete", middleware.RequireRole("admin"), remittanceHandler.CompleteRemittance)

			protected.POST("/invoices", remittanceHandler.CreateInvoice)
			protected.GET("/invoices", remittanceHandler.ListInvoices)
			protected.GET("/invoices/:id", remittanceHandler.GetInvoice)

			feeService := services.NewFeeService(cfg)
			feeHandler := handlers.NewFeeHandler(feeService)
			protected.GET("/fees/calculate", feeHandler.Calculate)

			exchangeRateHandler := handlers.NewExchangeRateHandler(cfg)
			protected.GET("/exchange-rates", middleware.RateLimitMiddleware(cfg), exchangeRateHandler.GetRate)

			auditHandler := handlers.NewAuditLogHandler(db)
			protected.GET("/audit/logs", middleware.RequireRole("admin"), auditHandler.List)

			exportHandler := handlers.NewExportHandler(db)
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

			// Saved recipients. The handler, its tests and the OpenAPI spec
			// existed, but the routes were never registered (#379).
			contactHandler := handlers.NewContactHandler(db, cfg, utils.NewStellarClient(cfg.HorizonURL, cfg.NetworkPassphrase))
			protected.POST("/contacts", contactHandler.CreateContact)
			protected.GET("/contacts", contactHandler.ListContacts)
			protected.GET("/contacts/:id", contactHandler.GetContact)
			protected.PUT("/contacts/:id", contactHandler.UpdateContact)
			protected.DELETE("/contacts/:id", contactHandler.DeleteContact)
			protected.POST("/contacts/:id/verify", contactHandler.VerifyContact)

			analyticsHandler := handlers.NewAnalyticsHandler(db)
			protected.GET("/analytics/volume", middleware.RequireRole("admin"), analyticsHandler.GetVolumeMetrics)
			protected.GET("/analytics/fees", middleware.RequireRole("admin"), analyticsHandler.GetFeeMetrics)
			protected.GET("/analytics/success-rate", middleware.RequireRole("admin"), analyticsHandler.GetSuccessRate)
			protected.GET("/analytics/top-corridors", middleware.RequireRole("admin"), analyticsHandler.GetTopCorridors)
			protected.GET("/analytics/sla", middleware.RequireRole("admin"), analyticsHandler.GetSLAMetrics)

		}
	}

	return router
}
