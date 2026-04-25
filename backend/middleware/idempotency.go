package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

// IdempotencyConfig holds configuration for idempotency middleware
type IdempotencyConfig struct {
	TTL           time.Duration // Time to live for idempotency keys (default: 24 hours)
	AllowedMethods []string      // HTTP methods that require idempotency keys
}

// DefaultIdempotencyConfig returns default configuration
func DefaultIdempotencyConfig() IdempotencyConfig {
	return IdempotencyConfig{
		TTL:           24 * time.Hour,
		AllowedMethods: []string{"POST", "PUT", "PATCH"},
	}
}

var (
	// In-memory cache for concurrent request handling
	// Key: idempotency key, Value: request context
	idempotencyCache = &sync.Map{}
)

// IdempotencyMiddleware creates middleware for handling idempotency keys
func IdempotencyMiddleware(db *gorm.DB, config ...IdempotencyConfig) gin.HandlerFunc {
	cfg := DefaultIdempotencyConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(c *gin.Context) {
		// Only apply to configured methods
		if !isMethodAllowed(c.Request.Method, cfg.AllowedMethods) {
			c.Next()
			return
		}

		// Get idempotency key from header
		idempotencyKey := c.GetHeader("Idempotency-Key")
		if idempotencyKey == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Idempotency-Key header is required for this request",
			})
			return
		}

		// Validate idempotency key format
		if err := validateIdempotencyKey(idempotencyKey); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Read request body for hashing
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Failed to read request body",
			})
			return
		}

		// Restore body for downstream handlers
		c.Request.Body = io.NopCloser(bytesBufferPool(bodyBytes))

		// Calculate request hash
		requestHash := calculateRequestHash(bodyBytes)

		// Check for existing idempotency record
		var existingRecord models.IdempotencyRecord
		result := db.Where("idempotency_key = ?", idempotencyKey).First(&existingRecord)

		if result.Error == nil {
			// Record exists - check if it's the same request
			if existingRecord.RequestHash != requestHash {
				// Different request body with same key - return 409
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{
					"error":         "Idempotency key already used with different request body",
					"existing_hash": existingRecord.RequestHash,
					"new_hash":      requestHash,
				})
				return
			}

			// Same request - check if response is still being processed
			if existingRecord.Status == "processing" {
				// Handle concurrent request - wait for response
				handleConcurrentRequest(c, idempotencyKey, &existingRecord, db)
				return
			}

			// Return cached response
			c.Header("X-Idempotent-Replayed", "true")
			c.Header("X-Idempotency-Key", idempotencyKey)

			if existingRecord.ResponseStatus > 0 {
				c.Status(existingRecord.ResponseStatus)
			}

			if existingRecord.ResponseBody != "" {
				var responseBody interface{}
				if err := json.Unmarshal([]byte(existingRecord.ResponseBody), &responseBody); err == nil {
					c.JSON(existingRecord.ResponseStatus, responseBody)
				} else {
					c.JSON(existingRecord.ResponseStatus, gin.H{
						"message": existingRecord.ResponseBody,
					})
				}
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"message": "Cached response",
			})
			return
		}

		// Create new idempotency record
		record := models.IdempotencyRecord{
			IdempotencyKey: idempotencyKey,
			RequestHash:    requestHash,
			RequestMethod:  c.Request.Method,
			RequestPath:    c.Request.URL.Path,
			Status:         "processing",
			CreatedAt:      time.Now(),
			ExpiresAt:      time.Now().Add(cfg.TTL),
		}

		if err := db.Create(&record).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to create idempotency record",
			})
			return
		}

		// Store in memory cache for concurrent handling
		idempotencyCache.Store(idempotencyKey, &concurrentRequestContext{
			mu:           &sync.Mutex{},
			completed:    false,
			responseBody: "",
			statusCode:   0,
		})

		// Set context for handlers to access
		c.Set("idempotency_key", idempotencyKey)
		c.Set("idempotency_record", &record)

		// Continue to handler
		c.Next()

		// After handler completes, update the record
		updateIdempotencyRecord(c, idempotencyKey, db)
	}
}

// isMethodAllowed checks if the HTTP method requires idempotency
func isMethodAllowed(method string, allowedMethods []string) bool {
	for _, m := range allowedMethods {
		if strings.EqualFold(method, m) {
			return true
		}
	}
	return false
}

// validateIdempotencyKey validates the format of the idempotency key
func validateIdempotencyKey(key string) error {
	if len(key) < 16 {
		return fmt.Errorf("idempotency key must be at least 16 characters")
	}
	if len(key) > 256 {
		return fmt.Errorf("idempotency key must not exceed 256 characters")
	}
	return nil
}

// calculateRequestHash creates a SHA256 hash of the request body
func calculateRequestHash(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

// concurrentRequestContext holds context for concurrent request handling
type concurrentRequestContext struct {
	mu           *sync.Mutex
	completed    bool
	responseBody string
	statusCode   int
}

// handleConcurrentRequest handles concurrent requests with the same idempotency key
func handleConcurrentRequest(c *gin.Context, key string, record *models.IdempotencyRecord, db *gorm.DB) {
	// Wait for the original request to complete
	maxWaitTime := 30 * time.Second
	pollInterval := 100 * time.Millisecond
	startTime := time.Now()

	for {
		// Check if original request completed
		var updatedRecord models.IdempotencyRecord
		if err := db.Where("idempotency_key = ?", key).First(&updatedRecord).Error; err != nil {
			break
		}

		if updatedRecord.Status == "completed" {
			// Return cached response
			c.Header("X-Idempotent-Replayed", "true")
			c.Header("X-Idempotency-Key", key)

			if updatedRecord.ResponseStatus > 0 {
				c.Status(updatedRecord.ResponseStatus)
			}

			if updatedRecord.ResponseBody != "" {
				var responseBody interface{}
				if err := json.Unmarshal([]byte(updatedRecord.ResponseBody), &responseBody); err == nil {
					c.JSON(updatedRecord.ResponseStatus, responseBody)
				} else {
					c.JSON(updatedRecord.ResponseStatus, gin.H{
						"message": updatedRecord.ResponseBody,
					})
				}
			}
			return
		}

		// Check timeout
		if time.Since(startTime) > maxWaitTime {
			c.AbortWithStatusJSON(http.StatusRequestTimeout, gin.H{
				"error": "Request with same idempotency key is still processing",
			})
			return
		}

		// Wait before next poll
		time.Sleep(pollInterval)
	}
}

// updateIdempotencyRecord updates the idempotency record after request completion
func updateIdempotencyRecord(c *gin.Context, key string, db *gorm.DB) {
	var record models.IdempotencyRecord
	if err := db.Where("idempotency_key = ?", key).First(&record).Error; err != nil {
		return
	}

	// Get response status
	statusCode := c.Writer.Status()
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	// Get response body
	responseBody := ""
	if rw, ok := c.Writer.(*gin.ResponseWriter); ok {
		// Try to capture response from context if stored
		if val, exists := c.Get("idempotency_response"); exists {
			if resp, ok := val.(string); ok {
				responseBody = resp
			}
		}
	}

	// Update record
	record.Status = "completed"
	record.ResponseStatus = statusCode
	record.ResponseBody = responseBody
	record.CompletedAt = time.Now()

	db.Save(&record)

	// Update in-memory cache
	if val, ok := idempotencyCache.Load(key); ok {
		ctx := val.(*concurrentRequestContext)
		ctx.mu.Lock()
		ctx.completed = true
		ctx.responseBody = responseBody
		ctx.statusCode = statusCode
		ctx.mu.Unlock()
	}
}

// SetIdempotencyResponse sets the response body for idempotency caching
func SetIdempotencyResponse(c *gin.Context, response interface{}) {
	// Store response in context for middleware to capture
	if responseBody, err := json.Marshal(response); err == nil {
		c.Set("idempotency_response", string(responseBody))
	}
}

// bytesBufferPool reuses byte buffers to reduce allocations
func bytesBufferPool(b []byte) *strings.Reader {
	return strings.NewReader(string(b))
}

// CleanupExpiredIdempotencyRecords removes expired idempotency records
func CleanupExpiredIdempotencyRecords(db *gorm.DB) error {
	return db.Where("expires_at < ?", time.Now()).Delete(&models.IdempotencyRecord{}).Error
}

// StartIdempotencyCleanupScheduler starts a background job to clean up expired records
func StartIdempotencyCleanupScheduler(db *gorm.DB, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			if err := CleanupExpiredIdempotencyRecords(db); err != nil {
				fmt.Printf("Error cleaning up idempotency records: %v\n", err)
			}
		}
	}()
}