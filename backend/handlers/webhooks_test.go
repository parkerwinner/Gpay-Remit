package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWebhookTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&models.Webhook{}, &models.WebhookDelivery{}, &models.User{})
	return db
}

func TestCreateWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWebhookTestDB()
	handler := NewWebhookHandler(db)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	})
	router.POST("/webhooks", handler.CreateWebhook)

	payload := CreateWebhookRequest{
		URL:         "https://example.com/webhook",
		Events:      []string{"payment.completed", "payment.failed"},
		Description: "Test webhook",
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/webhooks", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "https://example.com/webhook", response["url"])
	assert.NotEmpty(t, response["secret"])
	assert.True(t, response["is_active"].(bool))
}

func TestListWebhooks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWebhookTestDB()
	handler := NewWebhookHandler(db)

	// Create test webhooks
	db.Create(&models.Webhook{
		UserID:   1,
		URL:      "https://example.com/webhook1",
		Secret:   "secret1",
		Events:   "payment.completed",
		IsActive: true,
	})
	db.Create(&models.Webhook{
		UserID:   1,
		URL:      "https://example.com/webhook2",
		Secret:   "secret2",
		Events:   "payment.failed",
		IsActive: false,
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	})
	router.GET("/webhooks", handler.ListWebhooks)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/webhooks", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, 2, len(response))
}

func TestGetWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWebhookTestDB()
	handler := NewWebhookHandler(db)

	webhook := models.Webhook{
		UserID:      1,
		URL:         "https://example.com/webhook",
		Secret:      "secret123",
		Events:      "payment.completed",
		IsActive:    true,
		Description: "Test webhook",
	}
	db.Create(&webhook)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	})
	router.GET("/webhooks/:id", handler.GetWebhook)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/webhooks/1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "https://example.com/webhook", response["url"])
	assert.Equal(t, "Test webhook", response["description"])
}

func TestUpdateWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWebhookTestDB()
	handler := NewWebhookHandler(db)

	webhook := models.Webhook{
		UserID:   1,
		URL:      "https://example.com/webhook",
		Secret:   "secret123",
		Events:   "payment.completed",
		IsActive: true,
	}
	db.Create(&webhook)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	})
	router.PUT("/webhooks/:id", handler.UpdateWebhook)

	isActive := false
	payload := UpdateWebhookRequest{
		URL:      "https://example.com/new-webhook",
		IsActive: &isActive,
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/webhooks/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "https://example.com/new-webhook", response["url"])
	assert.False(t, response["is_active"].(bool))
}

func TestDeleteWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWebhookTestDB()
	handler := NewWebhookHandler(db)

	webhook := models.Webhook{
		UserID:   1,
		URL:      "https://example.com/webhook",
		Secret:   "secret123",
		Events:   "payment.completed",
		IsActive: true,
	}
	db.Create(&webhook)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	})
	router.DELETE("/webhooks/:id", handler.DeleteWebhook)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/webhooks/1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify webhook is deleted
	var count int64
	db.Model(&models.Webhook{}).Where("id = ?", 1).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestGetWebhookDeliveries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWebhookTestDB()
	handler := NewWebhookHandler(db)

	webhook := models.Webhook{
		UserID:   1,
		URL:      "https://example.com/webhook",
		Secret:   "secret123",
		Events:   "payment.completed",
		IsActive: true,
	}
	db.Create(&webhook)

	delivery := models.WebhookDelivery{
		WebhookID:    webhook.ID,
		Event:        "payment.completed",
		Payload:      `{"test": "data"}`,
		Status:       "success",
		ResponseCode: 200,
		AttemptCount: 1,
	}
	db.Create(&delivery)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	})
	router.GET("/webhooks/:id/deliveries", handler.GetWebhookDeliveries)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/webhooks/1/deliveries", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []models.WebhookDelivery
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, 1, len(response))
	assert.Equal(t, "payment.completed", response[0].Event)
	assert.Equal(t, "success", response[0].Status)
}

func TestCreateWebhook_InvalidURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWebhookTestDB()
	handler := NewWebhookHandler(db)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	})
	router.POST("/webhooks", handler.CreateWebhook)

	payload := CreateWebhookRequest{
		URL:    "not-a-valid-url",
		Events: []string{"payment.completed"},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/webhooks", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetWebhook_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWebhookTestDB()
	handler := NewWebhookHandler(db)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	})
	router.GET("/webhooks/:id", handler.GetWebhook)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/webhooks/999", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGenerateSecret(t *testing.T) {
	secret1, err := generateSecret(32)
	assert.NoError(t, err)
	assert.Len(t, secret1, 64) // 32 bytes = 64 hex characters

	secret2, err := generateSecret(32)
	assert.NoError(t, err)
	assert.NotEqual(t, secret1, secret2) // Should be random
}
