package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

type RemittanceHandler struct {
	db     *gorm.DB
	config *config.Config
}

func NewRemittanceHandler(db *gorm.DB, cfg *config.Config) *RemittanceHandler {
	return &RemittanceHandler{
		db:     db,
		config: cfg,
	}
}

type SendRemittanceRequest struct {
	SenderID       uint    `json:"sender_id" binding:"required"`
	RecipientID    uint    `json:"recipient_id" binding:"required"`
	Amount         float64 `json:"amount" binding:"required,gt=0"`
	Currency       string  `json:"currency" binding:"required"`
	TargetCurrency string  `json:"target_currency"`
	Notes          string  `json:"notes"`
}

func (h *RemittanceHandler) SendRemittance(c *gin.Context) {
	var req SendRemittanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	payment := models.Payment{
		SenderID:       req.SenderID,
		RecipientID:    req.RecipientID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		TargetCurrency: req.TargetCurrency,
		Status:         "pending",
		Notes:          req.Notes,
	}

	if err := h.db.Create(&payment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payment"})
		return
	}

	// TODO: Call Soroban contract to initiate escrow
	// stellarClient := utils.NewStellarClient(h.config.HorizonURL, h.config.NetworkPassphrase)

	c.JSON(http.StatusCreated, payment)
}

func (h *RemittanceHandler) GetRemittance(c *gin.Context) {
	id := c.Param("id")
	var payment models.Payment

	if err := h.db.First(&payment, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment not found"})
		return
	}

	c.JSON(http.StatusOK, payment)
}

func (h *RemittanceHandler) ListRemittances(c *gin.Context) {
	var payments []models.Payment

	if err := h.db.Find(&payments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch payments"})
		return
	}

	c.JSON(http.StatusOK, payments)
}

func (h *RemittanceHandler) CompleteRemittance(c *gin.Context) {
	id := c.Param("id")
	var payment models.Payment

	if err := h.db.First(&payment, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Payment not found"})
		return
	}

	payment.Status = "completed"
	if err := h.db.Save(&payment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update payment"})
		return
	}

	c.JSON(http.StatusOK, payment)
}

type CreateInvoiceRequest struct {
	PaymentID   uint    `json:"payment_id" binding:"required"`
	IssuerID    uint    `json:"issuer_id" binding:"required"`
	RecipientID uint    `json:"recipient_id" binding:"required"`
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	Currency    string  `json:"currency" binding:"required"`
	Description string  `json:"description"`
}

func (h *RemittanceHandler) CreateInvoice(c *gin.Context) {
	var req CreateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	invoiceNo := fmt.Sprintf("INV-%d-%d", time.Now().Unix(), req.PaymentID)

	invoice := models.Invoice{
		PaymentID:   req.PaymentID,
		InvoiceNo:   invoiceNo,
		IssuerID:    req.IssuerID,
		RecipientID: req.RecipientID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Description: req.Description,
		Status:      "unpaid",
	}

	if err := h.db.Create(&invoice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invoice"})
		return
	}

	c.JSON(http.StatusCreated, invoice)
}

func (h *RemittanceHandler) GetInvoice(c *gin.Context) {
	id := c.Param("id")
	var invoice models.Invoice

	if err := h.db.Preload("Payment").First(&invoice, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invoice not found"})
		return
	}

	c.JSON(http.StatusOK, invoice)
}
