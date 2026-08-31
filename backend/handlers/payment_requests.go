package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/middleware"
	"github.com/yourusername/gpay-remit/models"
	"github.com/yourusername/gpay-remit/services"
	"gorm.io/gorm"
)

type PaymentRequestHandler struct {
	db           *gorm.DB
	fees         *services.FeeService
	emailService *services.EmailService
}

func NewPaymentRequestHandler(db *gorm.DB, fees *services.FeeService, emailService *services.EmailService) *PaymentRequestHandler {
	return &PaymentRequestHandler{
		db:           db,
		fees:         fees,
		emailService: emailService,
	}
}

type CreatePaymentRequestRequest struct {
	TargetUserID uint    `json:"target_user_id" binding:"required"`
	Amount       float64 `json:"amount" binding:"required,gt=0"`
	Currency     string  `json:"currency" binding:"required"`
	AssetCode    string  `json:"asset_code" binding:"required"`
	AssetIssuer  string  `json:"asset_issuer"`
	Description  string  `json:"description"`
	Reference    string  `json:"reference"`
	ExpiresInHours *int  `json:"expires_in_hours"`
	Notes        string  `json:"notes"`
}

func (h *PaymentRequestHandler) CreatePaymentRequest(c *gin.Context) {
	var req CreatePaymentRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	requesterID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	if requesterID.(uint) == req.TargetUserID {
		c.Error(errors.NewValidationError("Cannot request payment from yourself", nil))
		return
	}

	var targetUser models.User
	if err := h.db.First(&targetUser, req.TargetUserID).Error; err != nil {
		c.Error(errors.NewNotFoundError("Target user not found"))
		return
	}

	var expiresAt *time.Time
	if req.ExpiresInHours != nil && *req.ExpiresInHours > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresInHours) * time.Hour)
		expiresAt = &t
	} else {
		t := time.Now().Add(7 * 24 * time.Hour)
		expiresAt = &t
	}

	paymentRequest := models.PaymentRequest{
		RequesterID:  requesterID.(uint),
		TargetUserID: req.TargetUserID,
		Amount:       req.Amount,
		Currency:     req.Currency,
		AssetCode:    req.AssetCode,
		AssetIssuer:  req.AssetIssuer,
		Description:  req.Description,
		Reference:    req.Reference,
		Status:       "pending",
		ExpiresAt:    expiresAt,
		Notes:        req.Notes,
	}

	if err := models.CreatePaymentRequest(h.db, &paymentRequest); err != nil {
		c.Error(errors.NewInternalError("Failed to create payment request", err))
		return
	}

	middleware.SetIdempotencyResponse(c, paymentRequest)

	c.JSON(http.StatusCreated, paymentRequest)
}

func (h *PaymentRequestHandler) GetPaymentRequest(c *gin.Context) {
	id := c.Param("id")
	var paymentRequest models.PaymentRequest

	if err := h.db.First(&paymentRequest, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Payment request not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch payment request", err))
		}
		return
	}

	c.JSON(http.StatusOK, paymentRequest)
}

func (h *PaymentRequestHandler) ListPaymentRequests(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	status := c.Query("status")
	requests, err := models.ListPaymentRequestsForUser(h.db, userID.(uint), status)
	if err != nil {
		c.Error(errors.NewInternalError("Failed to fetch payment requests", err))
		return
	}

	c.JSON(http.StatusOK, requests)
}

type AcceptPaymentRequestRequest struct {
	SenderAccount string `json:"sender_account" binding:"required"`
}

func (h *PaymentRequestHandler) AcceptPaymentRequest(c *gin.Context) {
	id := c.Param("id")
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	var paymentRequest models.PaymentRequest
	if err := h.db.First(&paymentRequest, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Payment request not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch payment request", err))
		}
		return
	}

	if paymentRequest.TargetUserID != userID.(uint) {
		c.Error(errors.NewForbiddenError("Only the target user can accept this payment request"))
		return
	}

	if paymentRequest.Status != "pending" {
		c.Error(errors.NewValidationError("Payment request is not pending", fmt.Sprintf("Current status: %s", paymentRequest.Status)))
		return
	}

	if paymentRequest.ExpiresAt != nil && time.Now().After(*paymentRequest.ExpiresAt) {
		paymentRequest.Status = "expired"
		if err := models.UpdatePaymentRequest(h.db, &paymentRequest); err != nil {
			c.Error(errors.NewInternalError("Failed to update payment request", err))
			return
		}
		c.Error(errors.NewValidationError("Payment request has expired", nil))
		return
	}

	var req AcceptPaymentRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	feeBreakdown := h.fees.Calculate(paymentRequest.Amount)
	payment := models.Payment{
		SenderID:      userID.(uint),
		SenderAccount: req.SenderAccount,
		RecipientID:   paymentRequest.RequesterID,
		Amount:        paymentRequest.Amount,
		Currency:      paymentRequest.Currency,
		Status:        "pending",
		Fee:           feeBreakdown.TotalFee,
		PlatformFee:   feeBreakdown.PlatformFee,
		ForexFee:      feeBreakdown.ForexFee,
		ComplianceFee: feeBreakdown.ComplianceFee,
		NetworkFee:    feeBreakdown.NetworkFee,
		Notes:         paymentRequest.Notes,
	}

	if err := h.db.Create(&payment).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to create payment", err))
		return
	}

	now := time.Now()
	paymentRequest.Status = "accepted"
	paymentRequest.AcceptedAt = &now
	paymentRequest.PaymentID = &payment.ID

	if err := models.UpdatePaymentRequest(h.db, &paymentRequest); err != nil {
		c.Error(errors.NewInternalError("Failed to update payment request", err))
		return
	}

	go func() {
		var requester models.User
		if err := h.db.First(&requester, paymentRequest.RequesterID).Error; err == nil {
			h.emailService.SendPaymentRequestAcceptedEmail(&requester, &paymentRequest)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"payment_request": paymentRequest,
		"payment":         payment,
		"fee_breakdown":   feeBreakdown,
		"message":         "Payment request accepted. Payment has been initiated.",
	})
}

type RejectPaymentRequestRequest struct {
	Reason string `json:"reason"`
}

func (h *PaymentRequestHandler) RejectPaymentRequest(c *gin.Context) {
	id := c.Param("id")
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	var paymentRequest models.PaymentRequest
	if err := h.db.First(&paymentRequest, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Payment request not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch payment request", err))
		}
		return
	}

	if paymentRequest.TargetUserID != userID.(uint) {
		c.Error(errors.NewForbiddenError("Only the target user can reject this payment request"))
		return
	}

	if paymentRequest.Status != "pending" {
		c.Error(errors.NewValidationError("Payment request is not pending", fmt.Sprintf("Current status: %s", paymentRequest.Status)))
		return
	}

	var req RejectPaymentRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	now := time.Now()
	paymentRequest.Status = "rejected"
	paymentRequest.RejectedAt = &now
	paymentRequest.RejectionReason = req.Reason

	if err := models.UpdatePaymentRequest(h.db, &paymentRequest); err != nil {
		c.Error(errors.NewInternalError("Failed to update payment request", err))
		return
	}

	go func() {
		var requester models.User
		if err := h.db.First(&requester, paymentRequest.RequesterID).Error; err == nil {
			h.emailService.SendPaymentRequestRejectedEmail(&requester, &paymentRequest)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"payment_request": paymentRequest,
		"message":         "Payment request rejected.",
	})
}

func (h *PaymentRequestHandler) CancelPaymentRequest(c *gin.Context) {
	id := c.Param("id")
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	var paymentRequest models.PaymentRequest
	if err := h.db.First(&paymentRequest, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Payment request not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch payment request", err))
		}
		return
	}

	if paymentRequest.RequesterID != userID.(uint) {
		c.Error(errors.NewForbiddenError("Only the requester can cancel this payment request"))
		return
	}

	if paymentRequest.Status != "pending" {
		c.Error(errors.NewValidationError("Payment request is not pending", fmt.Sprintf("Current status: %s", paymentRequest.Status)))
		return
	}

	paymentRequest.Status = "cancelled"
	if err := models.UpdatePaymentRequest(h.db, &paymentRequest); err != nil {
		c.Error(errors.NewInternalError("Failed to update payment request", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_request": paymentRequest,
		"message":         "Payment request cancelled.",
	})
}
