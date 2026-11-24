package handlers

import (
	"net/http"

	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type EmailPreferencesHandler struct {
	db *gorm.DB
}

func NewEmailPreferencesHandler(db *gorm.DB) *EmailPreferencesHandler {
	return &EmailPreferencesHandler{db: db}
}

type UpdateEmailPreferencesRequest struct {
	TransactionConfirmations *bool `json:"transaction_confirmations"`
	Marketing                *bool `json:"marketing"`
	SecurityAlerts           *bool `json:"security_alerts"`
}

// GetEmailPreferences returns the authenticated user's email notification preferences.
func (h *EmailPreferencesHandler) GetEmailPreferences(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	var prefs models.EmailPreferences
	err := h.db.Where("user_id = ?", userID).First(&prefs).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			prefs = models.DefaultEmailPreferences(userID)
			if createErr := h.db.Create(&prefs).Error; createErr != nil {
				c.Error(errors.NewInternalError("Failed to initialize email preferences", createErr))
				return
			}
		} else {
			c.Error(errors.NewInternalError("Failed to fetch email preferences", err))
			return
		}
	}

	prefs.SecurityAlerts = true
	c.JSON(http.StatusOK, prefs)
}

// UpdateEmailPreferences updates the authenticated user's email preferences.
// Security alerts cannot be disabled; attempting to set security_alerts=false
// returns a 400 validation error.
func (h *EmailPreferencesHandler) UpdateEmailPreferences(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	var req UpdateEmailPreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.SecurityAlerts != nil && !*req.SecurityAlerts {
		c.Error(errors.NewValidationError("Invalid preference", "security alerts cannot be disabled"))
		return
	}

	var prefs models.EmailPreferences
	err := h.db.Where("user_id = ?", userID).First(&prefs).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			prefs = models.DefaultEmailPreferences(userID)
		} else {
			c.Error(errors.NewInternalError("Failed to fetch email preferences", err))
			return
		}
	}

	if req.TransactionConfirmations != nil {
		prefs.TransactionConfirmations = *req.TransactionConfirmations
	}
	if req.Marketing != nil {
		prefs.Marketing = *req.Marketing
	}
	prefs.SecurityAlerts = true

	if prefs.ID == 0 {
		if err := h.db.Create(&prefs).Error; err != nil {
			c.Error(errors.NewInternalError("Failed to save email preferences", err))
			return
		}
	} else {
		if err := h.db.Save(&prefs).Error; err != nil {
			c.Error(errors.NewInternalError("Failed to update email preferences", err))
			return
		}
	}

	c.JSON(http.StatusOK, prefs)
}
