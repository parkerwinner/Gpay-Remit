package handlers

import (
	"net/http"
	"time"

	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type GDPRHandler struct {
	db *gorm.DB
}

func NewGDPRHandler(db *gorm.DB) *GDPRHandler {
	return &GDPRHandler{db: db}
}

// gdprWebhookExport is a webhook configuration as exported to the data
// subject. The HMAC signing secret is deliberately omitted: it is a
// credential, not personal data, and exporting it would widen the blast
// radius of a leaked export file.
type gdprWebhookExport struct {
	ID          uint      `json:"id"`
	URL         string    `json:"url"`
	Events      models.WebhookEvents `json:"events"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// gdprInvoiceExport flattens models.Invoice so the embedded Payment struct
// (populated only via preload) is not emitted as a zero-value object. The
// payment itself is already included under "payments".
type gdprInvoiceExport struct {
	ID          uint       `json:"id"`
	PaymentID   uint       `json:"payment_id"`
	InvoiceNo   string     `json:"invoice_no"`
	IssuerID    uint       `json:"issuer_id"`
	RecipientID uint       `json:"recipient_id"`
	Amount      int64      `json:"amount"`
	Currency    string     `json:"currency"`
	DueDate     *time.Time `json:"due_date"`
	Status      string     `json:"status"`
	Description string     `json:"description"`
	PdfURL      string     `json:"pdf_url"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type gdprExportResponse struct {
	User            models.User             `json:"user"`
	Payments        []models.Payment        `json:"payments"`
	PaymentRequests []models.PaymentRequest `json:"payment_requests"`
	Invoices        []gdprInvoiceExport     `json:"invoices"`
	Contacts        []models.Contact        `json:"contacts"`
	Webhooks        []gdprWebhookExport     `json:"webhooks"`
	AuditLogs       []models.AuditLog       `json:"audit_logs"`
}

// ExportUserData returns all personal data held for the authenticated user
// in a machine-readable (JSON) format, per GDPR Article 20 (data portability).
//
// Covered: the user record, payments (sent or received), payment requests
// (requested or targeted), invoices (issued or received), contacts (with
// verification status), webhook configurations (without secrets), and the
// audit log entries recorded for the user.
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
	if err := h.db.Where("sender_id = ? OR recipient_id = ?", userID, userID).
		Order("id ASC").Find(&payments).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch payment history", err))
		return
	}

	var paymentRequests []models.PaymentRequest
	if err := h.db.Where("requester_id = ? OR target_user_id = ?", userID, userID).
		Order("id ASC").Find(&paymentRequests).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch payment requests", err))
		return
	}

	var invoiceRows []models.Invoice
	if err := h.db.Where("issuer_id = ? OR recipient_id = ?", userID, userID).
		Order("id ASC").Find(&invoiceRows).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch invoices", err))
		return
	}
	invoices := make([]gdprInvoiceExport, 0, len(invoiceRows))
	for _, inv := range invoiceRows {
		invoices = append(invoices, gdprInvoiceExport{
			ID:          inv.ID,
			PaymentID:   inv.PaymentID,
			InvoiceNo:   inv.InvoiceNo,
			IssuerID:    inv.IssuerID,
			RecipientID: inv.RecipientID,
			Amount:      inv.Amount,
			Currency:    inv.Currency,
			DueDate:     inv.DueDate,
			Status:      inv.Status,
			Description: inv.Description,
			PdfURL:      inv.PdfURL,
			CreatedAt:   inv.CreatedAt,
			UpdatedAt:   inv.UpdatedAt,
		})
	}

	var contacts []models.Contact
	if err := h.db.Where("user_id = ?", userID).Order("id ASC").Find(&contacts).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch contacts", err))
		return
	}

	var webhookRows []models.Webhook
	if err := h.db.Where("user_id = ?", userID).Order("id ASC").Find(&webhookRows).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch webhooks", err))
		return
	}
	webhooks := make([]gdprWebhookExport, 0, len(webhookRows))
	for _, wh := range webhookRows {
		webhooks = append(webhooks, gdprWebhookExport{
			ID:          wh.ID,
			URL:         wh.URL,
			Events:      wh.Events,
			Description: wh.Description,
			IsActive:    wh.IsActive,
			CreatedAt:   wh.CreatedAt,
			UpdatedAt:   wh.UpdatedAt,
		})
	}

	var auditLogs []models.AuditLog
	if err := h.db.Where("user_id = ?", userID).Order("id ASC").Find(&auditLogs).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch audit logs", err))
		return
	}

	_ = h.db.Create(&models.AuditLog{
		UserID:    &userID,
		Action:    "gdpr.data_export",
		Resource:  "user",
		IPAddress: c.ClientIP(),
	}).Error

	c.JSON(http.StatusOK, gdprExportResponse{
		User:            user,
		Payments:        payments,
		PaymentRequests: paymentRequests,
		Invoices:        invoices,
		Contacts:        contacts,
		Webhooks:        webhooks,
		AuditLogs:       auditLogs,
	})
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
