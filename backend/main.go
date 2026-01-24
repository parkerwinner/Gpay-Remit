package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/handlers"
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
		// Remittance endpoints
		remittanceHandler := handlers.NewRemittanceHandler(db, cfg)
		api.POST("/remittances", remittanceHandler.SendRemittance)
		api.GET("/remittances/:id", remittanceHandler.GetRemittance)
		api.GET("/remittances", remittanceHandler.ListRemittances)
		api.POST("/remittances/:id/complete", remittanceHandler.CompleteRemittance)

		// Invoice endpoints
		api.POST("/invoices", remittanceHandler.CreateInvoice)
		api.GET("/invoices/:id", remittanceHandler.GetInvoice)

		// User endpoints (stub)
		api.POST("/users", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "User creation endpoint"})
		})
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
