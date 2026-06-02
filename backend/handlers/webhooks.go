package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/models"
	"github.com/yourusername/gpay-remit/services"
	"gorm.io/gorm"
)

type WebhookHandler struct {
	db              *gorm.DB
	deliveryService *services.WebhookDeliveryService
}

func NewWebhookHandler(db *gorm.DB) *WebhookHandler {
	return &WebhookHandler{
		db:              db,
		deliveryService: services.NewWebhookDeliveryService(db),
	}
}

type CreateWebhookRequest struct {
	URL         string   `json:"url" binding:"required,url"`
	Events      []string `json:"events" binding:"required,min=1"`
	Description string   `json:"description"`
}

type UpdateWebhookRequest struct {
	URL         string   `json:"url" binding:"omitempty,url"`
	Events      []string `json:"events" binding:"omitempty,min=1"`
	Description string   `json:"description"`
	IsActive    *bool    `json:"is_active"`
}

// CreateWebhook creates a new webhook
func (h *WebhookHandler) CreateWebhook(c *gin.Context) {
	var req CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewAuthError("Unauthorized"))
		return
	}

	// Generate a random secret for HMAC
	secret, err := generateSecret(32)
	if err != nil {
		c.Error(errors.NewInternalError("Failed to generate webhook secret", err))
		return
	}

	webhook := models.Webhook{
		UserID:      userID.(uint),
		URL:         req.URL,
		Secret:      secret,
		Events:      strings.Join(req.Events, ","),
		IsActive:    true,
		Description: req.Description,
	}

	if err := h.db.Create(&webhook).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to create webhook", err))
		return
	}

	response := gin.H{
		"id":          webhook.ID,
		"url":         webhook.URL,
		"events":      req.Events,
		"description": webhook.Description,
		"is_active":   webhook.IsActive,
		"secret":      secret, // Return secret only on creation
		"created_at":  webhook.CreatedAt,
	}

	c.JSON(http.StatusCreated, response)
}

// ListWebhooks lists all webhooks for the authenticated user
func (h *WebhookHandler) ListWebhooks(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewAuthError("Unauthorized"))
		return
	}

	var webhooks []models.Webhook
	if err := h.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&webhooks).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch webhooks", err))
		return
	}

	// Format response without secrets
	response := make([]gin.H, len(webhooks))
	for i, webhook := range webhooks {
		response[i] = gin.H{
			"id":          webhook.ID,
			"url":         webhook.URL,
			"events":      strings.Split(webhook.Events, ","),
			"description": webhook.Description,
			"is_active":   webhook.IsActive,
			"created_at":  webhook.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, response)
}

// GetWebhook retrieves a specific webhook
func (h *WebhookHandler) GetWebhook(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewAuthError("Unauthorized"))
		return
	}

	id := c.Param("id")
	var webhook models.Webhook

	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&webhook).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Webhook not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch webhook", err))
		}
		return
	}

	response := gin.H{
		"id":          webhook.ID,
		"url":         webhook.URL,
		"events":      strings.Split(webhook.Events, ","),
		"description": webhook.Description,
		"is_active":   webhook.IsActive,
		"created_at":  webhook.CreatedAt,
	}

	c.JSON(http.StatusOK, response)
}

// UpdateWebhook updates a webhook
func (h *WebhookHandler) UpdateWebhook(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewAuthError("Unauthorized"))
		return
	}

	id := c.Param("id")
	var req UpdateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	var webhook models.Webhook
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&webhook).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Webhook not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch webhook", err))
		}
		return
	}

	// Update fields
	if req.URL != "" {
		webhook.URL = req.URL
	}
	if len(req.Events) > 0 {
		webhook.Events = strings.Join(req.Events, ",")
	}
	if req.Description != "" {
		webhook.Description = req.Description
	}
	if req.IsActive != nil {
		webhook.IsActive = *req.IsActive
	}

	if err := h.db.Save(&webhook).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to update webhook", err))
		return
	}

	response := gin.H{
		"id":          webhook.ID,
		"url":         webhook.URL,
		"events":      strings.Split(webhook.Events, ","),
		"description": webhook.Description,
		"is_active":   webhook.IsActive,
		"updated_at":  webhook.UpdatedAt,
	}

	c.JSON(http.StatusOK, response)
}

// DeleteWebhook deletes a webhook
func (h *WebhookHandler) DeleteWebhook(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewAuthError("Unauthorized"))
		return
	}

	id := c.Param("id")
	var webhook models.Webhook

	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&webhook).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Webhook not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch webhook", err))
		}
		return
	}

	if err := h.db.Delete(&webhook).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to delete webhook", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Webhook deleted successfully"})
}

// GetWebhookDeliveries retrieves delivery logs for a webhook
func (h *WebhookHandler) GetWebhookDeliveries(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewAuthError("Unauthorized"))
		return
	}

	id := c.Param("id")
	var webhook models.Webhook

	// Verify webhook belongs to user
	if err := h.db.Where("id = ? AND user_id = ?", id, userID).First(&webhook).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Webhook not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch webhook", err))
		}
		return
	}

	// Fetch deliveries
	var deliveries []models.WebhookDelivery
	if err := h.db.Where("webhook_id = ?", id).
		Order("created_at DESC").
		Limit(100).
		Find(&deliveries).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch webhook deliveries", err))
		return
	}

	c.JSON(http.StatusOK, deliveries)
}

// RetryWebhookDelivery retries a failed webhook delivery
func (h *WebhookHandler) RetryWebhookDelivery(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewAuthError("Unauthorized"))
		return
	}

	deliveryID := c.Param("delivery_id")
	var delivery models.WebhookDelivery

	if err := h.db.First(&delivery, deliveryID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Delivery not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch delivery", err))
		}
		return
	}

	// Verify webhook belongs to user
	var webhook models.Webhook
	if err := h.db.Where("id = ? AND user_id = ?", delivery.WebhookID, userID).First(&webhook).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Webhook not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch webhook", err))
		}
		return
	}

	// Reset delivery status for retry
	delivery.Status = "pending"
	delivery.AttemptCount = 0
	delivery.NextRetryAt = nil
	delivery.CompletedAt = nil
	if err := h.db.Save(&delivery).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to reset delivery", err))
		return
	}

	// Trigger retry
	go h.deliveryService.DeliverWebhook(&webhook, &delivery)

	c.JSON(http.StatusOK, gin.H{"message": "Webhook delivery retry initiated"})
}

// generateSecret generates a random hex string for webhook secrets
func generateSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
