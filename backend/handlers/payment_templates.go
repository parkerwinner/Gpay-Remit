package handlers

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/Gpay-Remit/gpay-remit/validators"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaymentTemplateHandler struct {
	db            *gorm.DB
	config        *config.Config
	stellarClient utils.StellarClientInterface
	fees          *services.FeeService
}

func NewPaymentTemplateHandler(db *gorm.DB, cfg *config.Config, stellarClient utils.StellarClientInterface) *PaymentTemplateHandler {
	return &PaymentTemplateHandler{
		db:            db,
		config:        cfg,
		stellarClient: stellarClient,
		fees:          services.NewFeeService(cfg),
	}
}

type CreatePaymentTemplateRequest struct {
	TemplateName     string  `json:"template_name" binding:"required"`
	RecipientAccount string  `json:"recipient_account" binding:"required"`
	Amount           float64 `json:"amount" binding:"required,gt=0"`
	AssetCode        string  `json:"asset_code" binding:"required"`
	AssetIssuer      string  `json:"asset_issuer"`
	Notes            string  `json:"notes"`
}

type UpdatePaymentTemplateRequest struct {
	TemplateName     string  `json:"template_name"`
	RecipientAccount string  `json:"recipient_account"`
	Amount           float64 `json:"amount"`
	AssetCode        string  `json:"asset_code"`
	AssetIssuer      string  `json:"asset_issuer"`
	Notes            string  `json:"notes"`
}

func (h *PaymentTemplateHandler) validateTemplateFields(ctx context.Context, name, recipient, assetCode string, amount float64, notes string) error {
	if strings.TrimSpace(name) == "" || len(name) > 255 {
		return fmt.Errorf("template_name must be between 1 and 255 characters")
	}
	if err := validators.ValidateStellarAddress(recipient); err != nil {
		return fmt.Errorf("invalid recipient_account: %w", err)
	}
	if h.stellarClient != nil {
		if err := h.stellarClient.ValidateAccount(ctx, recipient); err != nil {
			return fmt.Errorf("recipient account validation failed: %w", err)
		}
	}
	if err := validators.ValidateAmount(amount); err != nil {
		return err
	}
	if err := validators.ValidateCurrency(assetCode); err != nil {
		return err
	}
	if len(notes) > 500 {
		return fmt.Errorf("notes cannot exceed 500 characters")
	}
	return nil
}

func (h *PaymentTemplateHandler) CreateTemplate(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	var req CreatePaymentTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	req.TemplateName = strings.TrimSpace(req.TemplateName)
	req.RecipientAccount = strings.TrimSpace(req.RecipientAccount)
	req.AssetCode = strings.ToUpper(strings.TrimSpace(req.AssetCode))
	req.AssetIssuer = strings.TrimSpace(req.AssetIssuer)

	ctx := utils.WithRequestContext(c.Request.Context(), c.GetString("requestID"), userID)
	if err := h.validateTemplateFields(ctx, req.TemplateName, req.RecipientAccount, req.AssetCode, req.Amount, req.Notes); err != nil {
		c.Error(errors.NewValidationError(err.Error(), nil))
		return
	}

	tpl := models.PaymentTemplate{
		UserID:           userID,
		TemplateName:     req.TemplateName,
		RecipientAccount: req.RecipientAccount,
		Amount:           req.Amount,
		AssetCode:        req.AssetCode,
		AssetIssuer:      req.AssetIssuer,
		Notes:            req.Notes,
	}

	if err := models.CreatePaymentTemplate(h.db, &tpl); err != nil {
		c.Error(errors.NewInternalError("Failed to create payment template", err))
		return
	}

	c.JSON(http.StatusCreated, tpl)
}

func (h *PaymentTemplateHandler) ListTemplates(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
			pageSize = parsed
		}
	}

	var totalCount int64
	if err := h.db.Model(&models.PaymentTemplate{}).Where("user_id = ?", userID).Count(&totalCount).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to count payment templates", err))
		return
	}

	var templates []models.PaymentTemplate
	if err := h.db.Scopes(Paginate(c)).Where("user_id = ?", userID).Order("created_at DESC").Find(&templates).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to list payment templates", err))
		return
	}

	if paginationErr, ok := c.Get("pagination_error"); ok {
		c.Error(paginationErr.(error))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        templates,
		"page":        page,
		"page_size":   pageSize,
		"total_count": totalCount,
	})
}

func (h *PaymentTemplateHandler) GetTemplate(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.Error(errors.NewValidationError("Invalid template ID", nil))
		return
	}

	var tpl models.PaymentTemplate
	if err := h.db.First(&tpl, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Payment template not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch payment template", err))
		}
		return
	}

	if tpl.UserID != userID {
		c.Error(errors.NewForbiddenError("Access denied to this payment template"))
		return
	}

	c.JSON(http.StatusOK, tpl)
}

func (h *PaymentTemplateHandler) UpdateTemplate(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.Error(errors.NewValidationError("Invalid template ID", nil))
		return
	}

	var tpl models.PaymentTemplate
	if err := h.db.First(&tpl, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Payment template not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch payment template", err))
		}
		return
	}

	if tpl.UserID != userID {
		c.Error(errors.NewForbiddenError("Access denied to this payment template"))
		return
	}

	var req UpdatePaymentTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	if req.TemplateName != "" {
		tpl.TemplateName = strings.TrimSpace(req.TemplateName)
	}
	if req.RecipientAccount != "" {
		tpl.RecipientAccount = strings.TrimSpace(req.RecipientAccount)
	}
	if req.Amount != 0 {
		tpl.Amount = req.Amount
	}
	if req.AssetCode != "" {
		tpl.AssetCode = strings.ToUpper(strings.TrimSpace(req.AssetCode))
	}
	if req.AssetIssuer != "" {
		tpl.AssetIssuer = strings.TrimSpace(req.AssetIssuer)
	}
	if req.Notes != "" {
		tpl.Notes = req.Notes
	}

	ctx := utils.WithRequestContext(c.Request.Context(), c.GetString("requestID"), userID)
	if err := h.validateTemplateFields(ctx, tpl.TemplateName, tpl.RecipientAccount, tpl.AssetCode, tpl.Amount, tpl.Notes); err != nil {
		c.Error(errors.NewValidationError(err.Error(), nil))
		return
	}

	if err := models.UpdatePaymentTemplate(h.db, &tpl); err != nil {
		c.Error(errors.NewInternalError("Failed to update payment template", err))
		return
	}

	c.JSON(http.StatusOK, tpl)
}

func (h *PaymentTemplateHandler) DeleteTemplate(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.Error(errors.NewValidationError("Invalid template ID", nil))
		return
	}

	var tpl models.PaymentTemplate
	if err := h.db.First(&tpl, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Payment template not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch payment template", err))
		}
		return
	}

	if tpl.UserID != userID {
		c.Error(errors.NewForbiddenError("Access denied to this payment template"))
		return
	}

	if err := models.DeletePaymentTemplate(h.db, uint(id), userID); err != nil {
		c.Error(errors.NewInternalError("Failed to delete payment template", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Payment template deleted"})
}

func (h *PaymentTemplateHandler) SendFromTemplate(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.Error(errors.NewValidationError("Invalid template ID", nil))
		return
	}

	var sender models.User
	if err := h.db.First(&sender, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewUnauthorizedError("Authenticated user not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to verify sender", err))
		}
		return
	}
	if !sender.IsActive {
		c.Error(errors.NewForbiddenError("Sender account is inactive"))
		return
	}

	ctx := utils.WithRequestContext(c.Request.Context(), c.GetString("requestID"), userID)

	var payment *models.Payment
	var feeBreakdown services.FeeBreakdown
	var httpErr *errors.AppError

	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		var tpl models.PaymentTemplate
		q := tx
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.First(&tpl, uint(id)).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				httpErr = errors.NewNotFoundError("Payment template not found")
			} else {
				httpErr = errors.NewInternalError("Failed to fetch payment template", err)
			}
			return err
		}

		if tpl.UserID != userID {
			httpErr = errors.NewForbiddenError("Access denied to this payment template")
			return fmt.Errorf("forbidden")
		}

		// Re-validate template fields at send time.
		if err := h.validateTemplateFields(ctx, tpl.TemplateName, tpl.RecipientAccount, tpl.AssetCode, tpl.Amount, tpl.Notes); err != nil {
			httpErr = errors.NewValidationError(err.Error(), nil)
			return err
		}

		amountUnits := int64(math.Round(tpl.Amount * 10000000))
		if amountUnits <= 0 {
			httpErr = errors.NewValidationError("Template amount must be positive", nil)
			return fmt.Errorf("invalid amount")
		}

		params := services.RemittanceCreateParams{
			SenderID:         userID,
			SenderAccount:    sender.StellarAddress,
			RecipientAccount: tpl.RecipientAccount,
			Amount:           amountUnits,
			AssetCode:        tpl.AssetCode,
			Notes:            tpl.Notes,
		}
		var createErr error
		payment, feeBreakdown, createErr = services.CreateRemittanceInTx(tx, h.fees, params)
		if createErr != nil {
			httpErr = errors.NewInternalError("Failed to create remittance from template", createErr)
			return createErr
		}
		return nil
	})

	if txErr != nil {
		if httpErr != nil {
			c.Error(httpErr)
		} else {
			c.Error(errors.NewInternalError("Failed to send from template", txErr))
		}
		return
	}

	response := gin.H{
		"remittance_id": payment.ID,
		"template_id":   uint(id),
		"status":        payment.Status,
		"fee_breakdown": feeBreakdown,
		"payment":       payment,
	}

	middleware.SetIdempotencyResponse(c, response)

	c.JSON(http.StatusCreated, response)
}
