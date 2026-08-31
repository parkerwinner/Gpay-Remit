package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/models"
	"github.com/yourusername/gpay-remit/utils"
	"gorm.io/gorm"
)

type ContactHandler struct {
	db            *gorm.DB
	config        *config.Config
	stellarClient utils.StellarClientInterface
}

func NewContactHandler(db *gorm.DB, cfg *config.Config, stellarClient utils.StellarClientInterface) *ContactHandler {
	return &ContactHandler{
		db:            db,
		config:        cfg,
		stellarClient: stellarClient,
	}
}

type CreateContactRequest struct {
	Nickname       string `json:"nickname" binding:"required"`
	StellarAddress string `json:"stellar_address" binding:"required"`
	Currency       string `json:"currency"`
	Email          string `json:"email"`
	Notes          string `json:"notes"`
}

type UpdateContactRequest struct {
	Nickname       string `json:"nickname"`
	StellarAddress string `json:"stellar_address"`
	Currency       string `json:"currency"`
	Email          string `json:"email"`
	Notes          string `json:"notes"`
}

func (h *ContactHandler) CreateContact(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Authentication required"))
		return
	}

	var req CreateContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", nil))
		return
	}

	req.StellarAddress = strings.TrimSpace(req.StellarAddress)
	req.Nickname = strings.TrimSpace(req.Nickname)

	contact := models.Contact{
		UserID:             userID.(uint),
		Nickname:           req.Nickname,
		StellarAddress:     req.StellarAddress,
		Currency:           req.Currency,
		Email:              req.Email,
		Notes:              req.Notes,
		VerificationStatus: models.ContactVerificationPending,
		IsVerified:         false,
	}

	if contact.Currency == "" {
		contact.Currency = "USDC"
	}

	if err := contact.Validate(); err != nil {
		c.Error(errors.NewValidationError(err.Error(), nil))
		return
	}

	// Verify Stellar address on-chain if stellar client available
	if h.stellarClient != nil {
		if err := h.stellarClient.ValidateAccount(c.Request.Context(), contact.StellarAddress); err == nil {
			contact.VerificationStatus = models.ContactVerificationVerified
			contact.IsVerified = true
		}
	}

	if err := h.db.Create(&contact).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to save contact", err))
		return
	}

	c.JSON(http.StatusCreated, contact)
}

func (h *ContactHandler) ListContacts(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Authentication required"))
		return
	}

	var contacts []models.Contact
	query := h.db.Where("user_id = ?", userID.(uint))

	if search := strings.TrimSpace(c.Query("search")); search != "" {
		searchPattern := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(nickname) LIKE ? OR LOWER(stellar_address) LIKE ? OR LOWER(email) LIKE ?", searchPattern, searchPattern, searchPattern)
	}

	if err := query.Order("created_at DESC").Find(&contacts).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch contacts", err))
		return
	}

	c.JSON(http.StatusOK, contacts)
}

func (h *ContactHandler) GetContact(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Authentication required"))
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.Error(errors.NewValidationError("Invalid contact ID", nil))
		return
	}

	var contact models.Contact
	if err := h.db.Where("id = ? AND user_id = ?", uint(id), userID.(uint)).First(&contact).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Contact not found"))
			return
		}
		c.Error(errors.NewInternalError("Failed to fetch contact", err))
		return
	}

	c.JSON(http.StatusOK, contact)
}

func (h *ContactHandler) UpdateContact(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Authentication required"))
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.Error(errors.NewValidationError("Invalid contact ID", nil))
		return
	}

	var contact models.Contact
	if err := h.db.Where("id = ? AND user_id = ?", uint(id), userID.(uint)).First(&contact).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Contact not found"))
			return
		}
		c.Error(errors.NewInternalError("Failed to fetch contact", err))
		return
	}

	var req UpdateContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", nil))
		return
	}

	if req.Nickname != "" {
		contact.Nickname = strings.TrimSpace(req.Nickname)
	}
	if req.StellarAddress != "" {
		contact.StellarAddress = strings.TrimSpace(req.StellarAddress)
		// re-validate address
		if err := contact.Validate(); err != nil {
			c.Error(errors.NewValidationError(err.Error(), nil))
			return
		}
		contact.VerificationStatus = models.ContactVerificationPending
		contact.IsVerified = false
		if h.stellarClient != nil {
			if err := h.stellarClient.ValidateAccount(c.Request.Context(), contact.StellarAddress); err == nil {
				contact.VerificationStatus = models.ContactVerificationVerified
				contact.IsVerified = true
			}
		}
	}
	if req.Currency != "" {
		contact.Currency = req.Currency
	}
	if req.Email != "" {
		contact.Email = req.Email
	}
	if req.Notes != "" {
		contact.Notes = req.Notes
	}

	if err := h.db.Save(&contact).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to update contact", err))
		return
	}

	c.JSON(http.StatusOK, contact)
}

func (h *ContactHandler) DeleteContact(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Authentication required"))
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.Error(errors.NewValidationError("Invalid contact ID", nil))
		return
	}

	var contact models.Contact
	if err := h.db.Where("id = ? AND user_id = ?", uint(id), userID.(uint)).First(&contact).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Contact not found"))
			return
		}
		c.Error(errors.NewInternalError("Failed to find contact", err))
		return
	}

	if err := h.db.Delete(&contact).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to delete contact", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Contact deleted successfully"})
}

func (h *ContactHandler) VerifyContact(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Authentication required"))
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.Error(errors.NewValidationError("Invalid contact ID", nil))
		return
	}

	var contact models.Contact
	if err := h.db.Where("id = ? AND user_id = ?", uint(id), userID.(uint)).First(&contact).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Contact not found"))
			return
		}
		c.Error(errors.NewInternalError("Failed to find contact", err))
		return
	}

	if h.stellarClient != nil {
		if err := h.stellarClient.ValidateAccount(c.Request.Context(), contact.StellarAddress); err != nil {
			contact.VerificationStatus = models.ContactVerificationFailed
			contact.IsVerified = false
			h.db.Save(&contact)
			c.JSON(http.StatusOK, gin.H{"verified": false, "status": "failed", "contact": contact})
			return
		}
	}

	contact.VerificationStatus = models.ContactVerificationVerified
	contact.IsVerified = true
	h.db.Save(&contact)

	c.JSON(http.StatusOK, gin.H{"verified": true, "status": "verified", "contact": contact})
}
