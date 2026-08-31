package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/middleware"
	"github.com/yourusername/gpay-remit/models"
	"github.com/yourusername/gpay-remit/services"
	"github.com/yourusername/gpay-remit/utils"
	"gorm.io/gorm"
)

type RemittanceHandler struct {
	db            *gorm.DB
	config        *config.Config
	stellarClient utils.StellarClientInterface
	fees          *services.FeeService
	emailService  *services.EmailService
}

func NewRemittanceHandler(db *gorm.DB, cfg *config.Config) *RemittanceHandler {
	return &RemittanceHandler{
		db:            db,
		config:        cfg,
		stellarClient: utils.NewStellarClient(cfg.HorizonURL, cfg.NetworkPassphrase),
		fees:          services.NewFeeService(cfg),
		emailService:  services.NewEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom, cfg.EmailEnabled),
	}
}

const (
	// MaxPage prevents integer overflow in offset calculation (#198)
	MaxPage = 10000
)

// PaginationCursor represents cursor-based pagination state
type PaginationCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uint      `json:"id"`
}

// EncodeCursor encodes pagination cursor to base64 string
func EncodeCursor(cursor PaginationCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeCursor decodes base64 cursor string to PaginationCursor
func DecodeCursor(encoded string) (PaginationCursor, error) {
	var cursor PaginationCursor
	if encoded == "" {
		return cursor, nil
	}
	
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return cursor, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	
	err = json.Unmarshal(data, &cursor)
	if err != nil {
		return cursor, fmt.Errorf("invalid cursor format: %w", err)
	}
	
	return cursor, nil
}

// Paginate is a GORM scope for pagination with overflow protection
func Paginate(c *gin.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		page := 1
		pageSize := 20

		if p := c.Query("page"); p != "" {
			fmt.Sscanf(p, "%d", &page)
		}
		if ps := c.Query("page_size"); ps != "" {
			fmt.Sscanf(ps, "%d", &pageSize)
		}

		if page <= 0 {
			page = 1
		}
		if page > MaxPage {
			// Return error context to be handled by calling function
			c.Set("pagination_error", errors.NewValidationError(fmt.Sprintf("Page number cannot exceed %d", MaxPage), nil))
			return db
		}
		if pageSize <= 0 || pageSize > 100 {
			pageSize = 20
		}

		offset := (page - 1) * pageSize
		return db.Offset(offset).Limit(pageSize)
	}
}

type CreateRemittanceRequest struct {
	SenderAccount   string                 `json:"sender_account" binding:"required"`
	RecipientAccount string                `json:"recipient_account" binding:"required"`
	Amount          float64                `json:"amount" binding:"required,gt=0"`
	AssetCode       string                 `json:"asset_code" binding:"required"`
	AssetIssuer     string                 `json:"asset_issuer"`
	Conditions      map[string]interface{} `json:"conditions"`
	Notes           string                 `json:"notes"`
}

type BatchPaymentItem struct {
	RecipientAccount string                 `json:"recipient_account" binding:"required"`
	Amount           float64                `json:"amount" binding:"required,gt=0"`
	Conditions       map[string]interface{} `json:"conditions"`
	Notes            string                 `json:"notes"`
}

type CreateBatchRemittanceRequest struct {
	SenderAccount string             `json:"sender_account" binding:"required"`
	AssetCode     string             `json:"asset_code" binding:"required"`
	AssetIssuer   string             `json:"asset_issuer"`
	Payments      []BatchPaymentItem `json:"payments" binding:"required,min=1,max=100"`
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
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	feeBreakdown := h.fees.Calculate(req.Amount)
	payment := models.Payment{
		SenderID:       req.SenderID,
		RecipientID:    req.RecipientID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		TargetCurrency: req.TargetCurrency,
		Status:         "pending",
		Fee:            feeBreakdown.TotalFee,
		PlatformFee:    feeBreakdown.PlatformFee,
		ForexFee:       feeBreakdown.ForexFee,
		ComplianceFee:  feeBreakdown.ComplianceFee,
		NetworkFee:     feeBreakdown.NetworkFee,
		Notes:          req.Notes,
	}

	if err := h.db.Create(&payment).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to create payment", err))
		return
	}

	// Set response for idempotency caching
	middleware.SetIdempotencyResponse(c, payment)

	c.JSON(http.StatusCreated, payment)
}

func (h *RemittanceHandler) CreateRemittance(c *gin.Context) {
	var req CreateRemittanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	ctx := utils.WithRequestContext(c.Request.Context(), c.GetString("requestID"), nil)

	// Validate Stellar accounts
	if err := h.stellarClient.ValidateAccount(ctx, req.SenderAccount); err != nil {
		c.Error(errors.NewValidationError("Invalid sender account", err.Error()))
		return
	}
	if err := h.stellarClient.ValidateAccount(ctx, req.RecipientAccount); err != nil {
		c.Error(errors.NewValidationError("Invalid recipient account", err.Error()))
		return
	}

	// Auth: Extract sender user ID from context (set by JWT middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	ctx = utils.WithRequestContext(ctx, c.GetString("requestID"), userID)

	// For simplicity, we'll assume the recipient user exists or we just store the account
	// In a real app, we'd lookup or create the recipient user.
	// For now, we'll just set RecipientID to 0 if not found, or use a placeholder.

	conditionsJSON, _ := json.Marshal(req.Conditions)

	feeBreakdown := h.fees.Calculate(req.Amount)
	payment := models.Payment{
		SenderID:         userID.(uint),
		SenderAccount:    req.SenderAccount,
		RecipientAccount: req.RecipientAccount,
		Amount:           req.Amount,
		Currency:         req.AssetCode,
		Status:           "pending",
		Fee:              feeBreakdown.TotalFee,
		PlatformFee:      feeBreakdown.PlatformFee,
		ForexFee:         feeBreakdown.ForexFee,
		ComplianceFee:    feeBreakdown.ComplianceFee,
		NetworkFee:       feeBreakdown.NetworkFee,
		Conditions:       string(conditionsJSON),
		Notes:            req.Notes,
	}

	var xdr string
	err := h.db.Transaction(func(tx *gorm.DB) error {
		// DB Save
		if err := tx.Create(&payment).Error; err != nil {
			return err
		}

		// Stellar Integration: Build escrow transaction envelope
		var stellarErr error
		xdr, stellarErr = h.stellarClient.BuildEscrowTx(
			ctx,
			req.SenderAccount,
			req.RecipientAccount,
			req.AssetCode,
			req.AssetIssuer,
			fmt.Sprintf("%.7f", req.Amount),
		)
		return stellarErr
	})

	if err != nil {
		c.Error(errors.NewInternalError("Failed to create remittance or build transaction", err))
		return
	}

	response := gin.H{
		"remittance_id": payment.ID,
		"status":        payment.Status,
		"fee_breakdown": feeBreakdown,
		"tx_envelope":   xdr,
		"message":       "Remittance initiated successfully. Please sign and submit the transaction.",
	}

	// Set response for idempotency caching
	middleware.SetIdempotencyResponse(c, response)

	c.JSON(http.StatusCreated, response)
}

func (h *RemittanceHandler) CreateBatchRemittance(c *gin.Context) {
	var req CreateBatchRemittanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	ctx := utils.WithRequestContext(c.Request.Context(), c.GetString("requestID"), nil)

	// Validate sender account
	if err := h.stellarClient.ValidateAccount(ctx, req.SenderAccount); err != nil {
		c.Error(errors.NewValidationError("Invalid sender account", err.Error()))
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	ctx = utils.WithRequestContext(ctx, c.GetString("requestID"), userID)

	var payments []models.Payment
	tx := h.db.Begin()
	if tx.Error != nil {
		c.Error(errors.NewInternalError("Failed to start transaction", tx.Error))
		return
	}

	for _, p := range req.Payments {
		if err := h.stellarClient.ValidateAccount(ctx, p.RecipientAccount); err != nil {
			tx.Rollback()
			c.Error(errors.NewValidationError("Invalid recipient account", fmt.Sprintf("Account %s is invalid", p.RecipientAccount)))
			return
		}

		conditionsJSON, _ := json.Marshal(p.Conditions)
		feeBreakdown := h.fees.Calculate(p.Amount)

		payment := models.Payment{
			SenderID:         userID.(uint),
			SenderAccount:    req.SenderAccount,
			RecipientAccount: p.RecipientAccount,
			Amount:           p.Amount,
			Currency:         req.AssetCode,
			Status:           "pending",
			Fee:              feeBreakdown.TotalFee,
			PlatformFee:      feeBreakdown.PlatformFee,
			ForexFee:         feeBreakdown.ForexFee,
			ComplianceFee:    feeBreakdown.ComplianceFee,
			NetworkFee:       feeBreakdown.NetworkFee,
			Conditions:       string(conditionsJSON),
			Notes:            p.Notes,
		}

		if err := tx.Create(&payment).Error; err != nil {
			tx.Rollback()
			c.Error(errors.NewInternalError("Failed to create remittance record", err))
			return
		}

		payments = append(payments, payment)
	}

	if err := tx.Commit().Error; err != nil {
		c.Error(errors.NewInternalError("Failed to commit transaction", err))
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Batch remittance initiated successfully",
		"payments": payments,
	})
}

func (h *RemittanceHandler) GetRemittance(c *gin.Context) {
	id := c.Param("id")
	var payment models.Payment

	if err := h.db.First(&payment, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Payment not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch payment", err))
		}
		return
	}

	c.JSON(http.StatusOK, payment)
}

type ListRemittancesResponse struct {
	Data       []models.Payment `json:"data"`
	Page       int              `json:"page,omitempty"`       // Deprecated: use cursor instead
	PageSize   int              `json:"page_size,omitempty"`  // Deprecated: use limit instead
	NextCursor string           `json:"next_cursor,omitempty"`
	HasMore    bool             `json:"has_more"`
	TotalCount *int64           `json:"total_count,omitempty"`
	HasNext    *bool            `json:"has_next,omitempty"`
	HasPrevious *bool           `json:"has_previous,omitempty"`
}

func (h *RemittanceHandler) ListRemittances(c *gin.Context) {
	var payments []models.Payment

	// Support both cursor-based and legacy offset-based pagination
	cursor := c.Query("cursor")
	limitStr := c.Query("limit")
	
	// Cursor-based pagination (preferred)
	if cursor != "" || limitStr != "" {
		// Cursor-based pagination eliminates overflow risk (#198)
		limit := 20
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
				limit = l
			}
		}

		decodedCursor, err := DecodeCursor(cursor)
		if err != nil {
			c.Error(errors.NewValidationError("Invalid cursor format", err.Error()))
			return
		}

		query := h.db.Model(&models.Payment{}).Order("created_at DESC, id DESC")
		
		// Apply cursor filtering: WHERE created_at < cursor.CreatedAt OR (created_at = cursor.CreatedAt AND id < cursor.ID)
		if !decodedCursor.CreatedAt.IsZero() {
			query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", 
				decodedCursor.CreatedAt, decodedCursor.CreatedAt, decodedCursor.ID)
		}

		if err := query.Limit(limit + 1).Find(&payments).Error; err != nil {
			c.Error(errors.NewInternalError("Failed to fetch payments", err))
			return
		}

		var nextCursor string
		hasMore := len(payments) > limit
		if hasMore {
			// Remove the extra item used for has_more detection
			lastItem := payments[limit-1]
			payments = payments[:limit]
			nextCursor = EncodeCursor(PaginationCursor{
				CreatedAt: lastItem.CreatedAt,
				ID:        lastItem.ID,
			})
		}

		response := ListRemittancesResponse{
			Data:       payments,
			NextCursor: nextCursor,
			HasMore:    hasMore,
		}

		c.JSON(http.StatusOK, response)
		return
	}

	// Legacy offset-based pagination for backward compatibility
	page := 1
	pageSize := 20
	fmt.Sscanf(c.Query("page"), "%d", &page)
	fmt.Sscanf(c.Query("page_size"), "%d", &pageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	// Cache key based on query params
	cacheKey := fmt.Sprintf("payments:list:%d:%d", page, pageSize)

	// Try cache
	if found, _ := utils.GetCached(cacheKey, &payments); found {
		c.Header("X-Cache", "HIT")
		c.JSON(http.StatusOK, ListRemittancesResponse{
			Data:     payments,
			Page:     page,
			PageSize: pageSize,
			HasMore:  len(payments) == pageSize,
		})
		return
	}

	// Count total records for metadata
	var totalCount int64
	h.db.Model(&models.Payment{}).Count(&totalCount)

	// DB query with pagination - check for pagination error from MaxPage validation
	if err := h.db.Scopes(Paginate(c)).Order("created_at DESC").Find(&payments).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch payments", err))
		return
	}

	// Check if MaxPage validation failed
	if paginationErr, exists := c.Get("pagination_error"); exists {
		c.Error(paginationErr.(error))
		return
	}

	// Set cache for 30 seconds
	utils.SetCached(cacheKey, payments, 30*time.Second)

	hasNext := int64(page*pageSize) < totalCount
	hasPrevious := page > 1

	c.Header("X-Cache", "MISS")
	c.JSON(http.StatusOK, ListRemittancesResponse{
		Data:        payments,
		Page:        page,
		PageSize:    pageSize,
		HasMore:     len(payments) == pageSize,
		TotalCount:  &totalCount,
		HasNext:     &hasNext,
		HasPrevious: &hasPrevious,
	})
}

func (h *RemittanceHandler) CompleteRemittance(c *gin.Context) {
	id := c.Param("id")
	var payment models.Payment

	if err := h.db.First(&payment, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Payment not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch payment", err))
		}
		return
	}

	middleware.SetAuditOld(c, payment)
	payment.Status = "completed"
	if err := h.db.Save(&payment).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to update payment", err))
		return
	}

	// Send email notification to sender
	var sender models.User
	if err := h.db.First(&sender, payment.SenderID).Error; err == nil {
		go h.emailService.SendPaymentCompletedEmail(&sender, &payment)
	}

	middleware.SetAuditNew(c, payment)
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
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
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
		c.Error(errors.NewInternalError("Failed to create invoice", err))
		return
	}

	// Set response for idempotency caching
	middleware.SetIdempotencyResponse(c, invoice)

	c.JSON(http.StatusCreated, invoice)
}

func (h *RemittanceHandler) GetInvoice(c *gin.Context) {
	id := c.Param("id")
	var invoice models.Invoice

	if err := h.db.Preload("Payment").First(&invoice, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Invoice not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch invoice", err))
		}
		return
	}

	c.JSON(http.StatusOK, invoice)
}

type ListInvoicesResponse struct {
	Data       []models.Invoice `json:"data"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalCount int64            `json:"total_count"`
}

func (h *RemittanceHandler) ListInvoices(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	page := 1
	pageSize := 20
	fmt.Sscanf(c.Query("page"), "%d", &page)
	fmt.Sscanf(c.Query("page_size"), "%d", &pageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	query := h.db.Model(&models.Invoice{}).
		Where("issuer_id = ? OR recipient_id = ?", userID, userID)

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if from := c.Query("from"); from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if to := c.Query("to"); to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			query = query.Where("created_at <= ?", t.Add(24*time.Hour-time.Second))
		}
	}

	var total int64
	query.Count(&total)

	var invoices []models.Invoice
	if err := query.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&invoices).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch invoices", err))
		return
	}

	// Batch-load associated payments in a single query (fixes N+1)
	if len(invoices) > 0 {
		paymentIDs := make([]uint, len(invoices))
		for i, inv := range invoices {
			paymentIDs[i] = inv.PaymentID
		}
		var payments []models.Payment
		if err := h.db.Where("id IN ?", paymentIDs).Find(&payments).Error; err != nil {
			c.Error(errors.NewInternalError("Failed to fetch payments for invoices", err))
			return
		}
		paymentMap := make(map[uint]models.Payment, len(payments))
		for _, p := range payments {
			paymentMap[p.ID] = p
		}
		for i := range invoices {
			if p, ok := paymentMap[invoices[i].PaymentID]; ok {
				invoices[i].Payment = p
			}
		}
	}

	c.JSON(http.StatusOK, ListInvoicesResponse{
		Data:       invoices,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
	})
}
