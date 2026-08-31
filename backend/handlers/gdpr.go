package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

type GDPRHandler struct {
	db *gorm.DB
}

func NewGDPRHandler(db *gorm.DB) *GDPRHandler {
	return &GDPRHandler{db: db}
}

type gdprExportResponse struct {
	User     models.User      `json:"user"`
	Payments []models.Payment `json:"payments"`
}

// ExportUserData returns all personal data held for the authenticated user
// in a machine-readable (JSON) format, per GDPR Article 20 (data portability).
func (h *GDPRHandler) ExportUserData(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Authentication required"))
		return
	}
	userID := userIDVal.(uint)

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		c.Error(errors.NewNotFoundError("User not found"))
		return
	}

	var payments []models.Payment
	if err := h.db.Where("sender_id = ? OR recipient_id = ?", userID, userID).Find(&payments).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch payment history", err))
		return
	}

	_ = h.db.Create(&models.AuditLog{
		UserID:    &userID,
		Action:    "gdpr.data_export",
		Resource:  "user",
		IPAddress: c.ClientIP(),
	}).Error

	c.JSON(http.StatusOK, gdprExportResponse{User: user, Payments: payments})
}

// DeleteAccount implements the GDPR right to erasure (Article 17). Financial
// records (payments) must be retained for regulatory/audit purposes, so
// rather than deleting the user row outright, personally identifying fields
// are anonymized and the account is soft-deleted; payment records are left
// intact but no longer resolve to identifying user data.
func (h *GDPRHandler) DeleteAccount(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Authentication required"))
		return
	}
	userID := userIDVal.(uint)

	anonymized := map[string]interface{}{
		"email":           "deleted-user-" + c.GetString("requestID") + "@anonymized.invalid",
		"name":            "Deleted User",
		"stellar_address": "",
		"country":         "",
		"is_active":       false,
		"preferences":     "{}",
	}

	if err := h.db.Model(&models.User{}).Where("id = ?", userID).Updates(anonymized).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to anonymize account", err))
		return
	}
	if err := h.db.Delete(&models.User{}, userID).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to delete account", err))
		return
	}

	_ = h.db.Create(&models.AuditLog{
		UserID:    &userID,
		Action:    "gdpr.account_deletion",
		Resource:  "user",
		IPAddress: c.ClientIP(),
	}).Error

	c.JSON(http.StatusOK, gin.H{"message": "Account deleted and personal data anonymized. Financial records retained per regulatory requirements."})
}
