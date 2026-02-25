package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/handlers"
	"github.com/yourusername/gpay-remit/middleware"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	db, err := config.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Setup router
	router := gin.Default()

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

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "gpay-remit-api",
		})
	})

	// API routes
	api := router.Group("/api/v1")
	{
		// Public auth endpoints
		authHandler := handlers.NewAuthHandler(db, cfg)
		api.POST("/auth/refresh", authHandler.Refresh)
		api.POST("/auth/login", func(c *gin.Context) {
			// Stub login endpoint
			c.JSON(http.StatusOK, gin.H{"message": "Login endpoint stub"})
		})
		
		// Public user endpoints
		api.POST("/users", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "User creation endpoint"})
		})

		// Protected routes
		protected := api.Group("/")
		protected.Use(middleware.JwtAuthMiddleware(cfg))
		{
			// Remittance endpoints
			remittanceHandler := handlers.NewRemittanceHandler(db, cfg)
			protected.POST("/remittances/create", remittanceHandler.CreateRemittance)
			protected.POST("/remittances", remittanceHandler.SendRemittance)
			protected.GET("/remittances/:id", remittanceHandler.GetRemittance)
			protected.GET("/remittances", remittanceHandler.ListRemittances)
			protected.POST("/remittances/:id/complete", middleware.RequireRole("admin"), remittanceHandler.CompleteRemittance)

			// Invoice endpoints
			protected.POST("/invoices", remittanceHandler.CreateInvoice)
			protected.GET("/invoices/:id", remittanceHandler.GetInvoice)
		}
	}

	// Start server
	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Gpay-Remit API server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
