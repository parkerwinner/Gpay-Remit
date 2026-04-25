package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

type AuditLogHandler struct {
	db *gorm.DB
}

func NewAuditLogHandler(db *gorm.DB) *AuditLogHandler {
	return &AuditLogHandler{db: db}
}

// List returns audit logs for admins only.
func (h *AuditLogHandler) List(c *gin.Context) {
	var logs []models.AuditLog

	// Simple pagination (reuse existing Paginate scope).
	if err := h.db.Scopes(Paginate(c)).Order("created_at DESC").Find(&logs).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch audit logs", err))
		return
	}

	c.JSON(http.StatusOK, logs)
}
