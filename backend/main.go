package main

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/handlers"
	"github.com/yourusername/gpay-remit/logger"
	"github.com/yourusername/gpay-remit/middleware"
	"github.com/yourusername/gpay-remit/utils"
)

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

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestLogger())

	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "gpay-remit-api",
		})
	})

	api := router.Group("/api/v1")
	{
		authHandler := handlers.NewAuthHandler(db, cfg)
		api.POST("/auth/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
		api.POST("/auth/refresh", authHandler.Refresh)

		api.POST("/users", authHandler.Register)

		protected := api.Group("/")
		protected.Use(middleware.JwtAuthMiddleware(cfg))
		{
			remittanceHandler := handlers.NewRemittanceHandler(db, cfg)
			protected.POST("/remittances/create", remittanceHandler.CreateRemittance)
			protected.POST("/remittances", remittanceHandler.SendRemittance)
			protected.GET("/remittances/:id", remittanceHandler.GetRemittance)
			protected.GET("/remittances", remittanceHandler.ListRemittances)
			protected.POST("/remittances/:id/complete", middleware.RequireRole("admin"), remittanceHandler.CompleteRemittance)

			protected.POST("/invoices", remittanceHandler.CreateInvoice)
			protected.GET("/invoices/:id", remittanceHandler.GetInvoice)
		}
	}

	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	logger.Log.WithField("port", port).Info("Starting Gpay-Remit API server")
	if err := router.Run(":" + port); err != nil {
		logger.Log.WithField("error", err).Fatal("Failed to start server")
	}
}
