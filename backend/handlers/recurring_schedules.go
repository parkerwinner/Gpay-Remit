package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/Gpay-Remit/gpay-remit/validators"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type RecurringScheduleHandler struct {
	db            *gorm.DB
	config        *config.Config
	stellarClient utils.StellarClientInterface
}

func NewRecurringScheduleHandler(db *gorm.DB, cfg *config.Config, stellarClient utils.StellarClientInterface) *RecurringScheduleHandler {
	return &RecurringScheduleHandler{
		db:            db,
		config:        cfg,
		stellarClient: stellarClient,
	}
}

type CreateRecurringScheduleRequest struct {
	SenderAccount    string     `json:"sender_account" binding:"required"`
	RecipientAccount string     `json:"recipient_account" binding:"required"`
	Amount           int64      `json:"amount" binding:"required,gt=0"`
	Currency         string     `json:"currency" binding:"required"`
	AssetIssuer      string     `json:"asset_issuer"`
	Frequency        string     `json:"frequency" binding:"required"`
	NextRunAt        *time.Time `json:"next_run_at"`
	Notes            string     `json:"notes"`
}

func (h *RecurringScheduleHandler) CreateSchedule(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	var req CreateRecurringScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	req.SenderAccount = strings.TrimSpace(req.SenderAccount)
	req.RecipientAccount = strings.TrimSpace(req.RecipientAccount)
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	req.Frequency = strings.ToLower(strings.TrimSpace(req.Frequency))

	if !models.IsValidRecurringFrequency(req.Frequency) {
		c.Error(errors.NewValidationError("Invalid frequency (must be daily, weekly, or monthly)", nil))
		return
	}
	if err := validators.ValidateCurrency(req.Currency); err != nil {
		c.Error(errors.NewValidationError(err.Error(), nil))
		return
	}
	if err := validators.ValidateStellarAddress(req.SenderAccount); err != nil {
		c.Error(errors.NewValidationError("Invalid sender account", err.Error()))
		return
	}
	if err := validators.ValidateStellarAddress(req.RecipientAccount); err != nil {
		c.Error(errors.NewValidationError("Invalid recipient account", err.Error()))
		return
	}
	if len(req.Notes) > 500 {
		c.Error(errors.NewValidationError("Notes cannot exceed 500 characters", nil))
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewUnauthorizedError("Authenticated user not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to verify user", err))
		}
		return
	}
	if !user.IsActive {
		c.Error(errors.NewForbiddenError("User account is inactive"))
		return
	}
	if user.StellarAddress == "" || req.SenderAccount != user.StellarAddress {
		c.Error(errors.NewForbiddenError("Sender account does not belong to the authenticated user"))
		return
	}

	ctx := utils.WithRequestContext(c.Request.Context(), c.GetString("requestID"), userID)
	if h.stellarClient != nil {
		if err := h.stellarClient.ValidateAccount(ctx, req.RecipientAccount); err != nil {
			c.Error(errors.NewValidationError("Invalid recipient account", err.Error()))
			return
		}
	}

	now := time.Now().UTC()
	var nextRunAt time.Time
	if req.NextRunAt != nil {
		nextRunAt = req.NextRunAt.UTC()
		if !nextRunAt.After(now) {
			c.Error(errors.NewValidationError("next_run_at must be in the future (UTC)", nil))
			return
		}
	} else {
		computed, err := models.ComputeNextRunAt(now, req.Frequency)
		if err != nil {
			c.Error(errors.NewValidationError(err.Error(), nil))
			return
		}
		nextRunAt = computed
	}

	schedule := models.RecurringSchedule{
		UserID:           userID,
		SenderAccount:    req.SenderAccount,
		RecipientAccount: req.RecipientAccount,
		Amount:           req.Amount,
		Currency:         req.Currency,
		AssetIssuer:      strings.TrimSpace(req.AssetIssuer),
		Frequency:        req.Frequency,
		Status:           models.RecurringStatusActive,
		NextRunAt:        nextRunAt,
		Notes:            req.Notes,
	}

	if err := h.db.Create(&schedule).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to create recurring schedule", err))
		return
	}

	c.JSON(http.StatusCreated, schedule)
}

func (h *RecurringScheduleHandler) ListSchedules(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	var schedules []models.RecurringSchedule
	query := h.db.Where("user_id = ?", userID)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Scopes(Paginate(c)).Order("created_at DESC").Find(&schedules).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to list recurring schedules", err))
		return
	}
	if paginationErr, ok := c.Get("pagination_error"); ok {
		c.Error(paginationErr.(error))
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": schedules})
}

func (h *RecurringScheduleHandler) fetchOwnedSchedule(c *gin.Context) (*models.RecurringSchedule, bool) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return nil, false
	}
	userID := userIDVal.(uint)

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.Error(errors.NewValidationError("Invalid schedule ID", nil))
		return nil, false
	}

	var schedule models.RecurringSchedule
	if err := h.db.First(&schedule, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Recurring schedule not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch recurring schedule", err))
		}
		return nil, false
	}

	if schedule.UserID != userID {
		c.Error(errors.NewForbiddenError("Access denied to this recurring schedule"))
		return nil, false
	}

	return &schedule, true
}

func (h *RecurringScheduleHandler) GetSchedule(c *gin.Context) {
	schedule, ok := h.fetchOwnedSchedule(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, schedule)
}

func (h *RecurringScheduleHandler) PauseSchedule(c *gin.Context) {
	schedule, ok := h.fetchOwnedSchedule(c)
	if !ok {
		return
	}

	switch schedule.Status {
	case models.RecurringStatusPaused:
		c.JSON(http.StatusOK, schedule)
		return
	case models.RecurringStatusCancelled:
		c.Error(errors.NewValidationError("Cannot pause a cancelled schedule", nil))
		return
	case models.RecurringStatusActive:
		schedule.Status = models.RecurringStatusPaused
		if err := h.db.Save(schedule).Error; err != nil {
			c.Error(errors.NewInternalError("Failed to pause schedule", err))
			return
		}
		c.JSON(http.StatusOK, schedule)
	default:
		c.Error(errors.NewValidationError("Invalid schedule state transition", nil))
	}
}

func (h *RecurringScheduleHandler) ResumeSchedule(c *gin.Context) {
	schedule, ok := h.fetchOwnedSchedule(c)
	if !ok {
		return
	}

	switch schedule.Status {
	case models.RecurringStatusActive:
		c.JSON(http.StatusOK, schedule)
		return
	case models.RecurringStatusCancelled:
		c.Error(errors.NewValidationError("Cannot resume a cancelled schedule", nil))
		return
	case models.RecurringStatusPaused:
		now := time.Now().UTC()
		nextRun, err := models.NextFutureRunAt(schedule.NextRunAt, now, schedule.Frequency)
		if err != nil {
			c.Error(errors.NewInternalError("Failed to compute next run time", err))
			return
		}
		schedule.Status = models.RecurringStatusActive
		schedule.NextRunAt = nextRun
		schedule.ConsecutiveFailures = 0
		if err := h.db.Save(schedule).Error; err != nil {
			c.Error(errors.NewInternalError("Failed to resume schedule", err))
			return
		}
		c.JSON(http.StatusOK, schedule)
	default:
		c.Error(errors.NewValidationError("Invalid schedule state transition", nil))
	}
}

func (h *RecurringScheduleHandler) CancelSchedule(c *gin.Context) {
	schedule, ok := h.fetchOwnedSchedule(c)
	if !ok {
		return
	}

	if schedule.Status == models.RecurringStatusCancelled {
		c.JSON(http.StatusOK, schedule)
		return
	}

	schedule.Status = models.RecurringStatusCancelled
	if err := h.db.Save(schedule).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to cancel schedule", err))
		return
	}
	c.JSON(http.StatusOK, schedule)
}
