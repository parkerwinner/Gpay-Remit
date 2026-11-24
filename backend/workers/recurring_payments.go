package workers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/logger"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/Gpay-Remit/gpay-remit/validators"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StartRecurringPaymentWorker starts the background worker when enabled by configuration.
func StartRecurringPaymentWorker(ctx context.Context, wg *sync.WaitGroup, db *gorm.DB, cfg *config.Config) {
	interval := cfg.RecurringWorkerInterval
	if interval <= 0 {
		interval = time.Minute
	}
	fees := services.NewFeeService(cfg)
	stellarClient := utils.NewStellarClient(cfg.StellarHorizonURL, cfg.NetworkPassphrase)
	emailService := services.NewEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom, cfg.EmailEnabled)

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Log.WithField("interval", interval.String()).Info("Recurring payment worker started")

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Log.Info("Recurring payment worker stopped")
				return
			case <-ticker.C:
				runTimeout := cfg.RecurringWorkerTimeout
				if runTimeout <= 0 {
					runTimeout = 30 * time.Second
				}
				runCtx, cancel := context.WithTimeout(ctx, runTimeout)
				safeProcessRecurringTick(runCtx, db, cfg, fees, stellarClient, emailService)
				cancel()
			}
		}
	}()
}

func safeProcessRecurringTick(
	ctx context.Context,
	db *gorm.DB,
	cfg *config.Config,
	fees *services.FeeService,
	stellarClient utils.StellarClientInterface,
	emailService *services.EmailService,
) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.WithField("panic", fmt.Sprintf("%v", r)).Error("Recovered from panic in recurring payment worker")
		}
	}()

	if _, err := ProcessDueRecurringSchedules(ctx, db, cfg, fees, stellarClient, emailService, time.Now().UTC()); err != nil {
		logger.Log.WithField("error", err).Error("Recurring payment worker cycle failed")
	}
}

// ProcessDueRecurringSchedules claims and executes due active recurring schedules at most once
// per scheduled_for timestamp. Returns the number of schedules processed.
func ProcessDueRecurringSchedules(
	ctx context.Context,
	db *gorm.DB,
	cfg *config.Config,
	fees *services.FeeService,
	stellarClient utils.StellarClientInterface,
	emailService *services.EmailService,
	now time.Time,
) (int, error) {
	nowUTC := now.UTC()
	batchSize := cfg.RecurringWorkerBatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	maxFailures := cfg.RecurringMaxFailures
	if maxFailures <= 0 {
		maxFailures = 3
	}

	var dueIDs []uint
	if err := db.WithContext(ctx).
		Model(&models.RecurringSchedule{}).
		Where("status = ? AND next_run_at <= ?", models.RecurringStatusActive, nowUTC).
		Order("next_run_at ASC").
		Limit(batchSize).
		Pluck("id", &dueIDs).Error; err != nil {
		return 0, err
	}

	processed := 0
	for _, scheduleID := range dueIDs {
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}

		var failedUser *models.User
		var failedPayment *models.Payment
		var failureReason string

		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var schedule models.RecurringSchedule
			q := tx
			if tx.Dialector.Name() == "postgres" {
				q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
			}
			if err := q.Where("id = ? AND status = ? AND next_run_at <= ?", scheduleID, models.RecurringStatusActive, nowUTC).
				First(&schedule).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return nil // already claimed or status changed
				}
				return err
			}

			scheduledFor := schedule.NextRunAt.UTC()
			execRecord := models.RecurringScheduleExecution{
				ScheduleID:   schedule.ID,
				ScheduledFor: scheduledFor,
				Status:       "processing",
			}
			if err := tx.Create(&execRecord).Error; err != nil {
				// Unique constraint on (schedule_id, scheduled_for) prevents duplicate execution.
				if isUniqueConstraintErr(err) {
					return nil
				}
				return err
			}

			nextRunAt, err := models.NextFutureRunAt(scheduledFor, nowUTC, schedule.Frequency)
			if err != nil {
				return err
			}

			// Re-validate sender, recipient, amount, and currency.
			var user models.User
			if err := tx.First(&user, schedule.UserID).Error; err != nil {
				return recordScheduleFailure(tx, &schedule, &execRecord, nextRunAt, maxFailures, "sender user not found", &failureReason)
			}
			if !user.IsActive {
				failedUser = &user
				return recordScheduleFailure(tx, &schedule, &execRecord, nextRunAt, maxFailures, "sender account is inactive", &failureReason)
			}
			if user.StellarAddress == "" || schedule.SenderAccount != user.StellarAddress {
				failedUser = &user
				return recordScheduleFailure(tx, &schedule, &execRecord, nextRunAt, maxFailures, "sender stellar address mismatch", &failureReason)
			}
			if err := validators.ValidateCurrency(schedule.Currency); err != nil {
				failedUser = &user
				return recordScheduleFailure(tx, &schedule, &execRecord, nextRunAt, maxFailures, err.Error(), &failureReason)
			}
			if err := validators.ValidateStellarAddress(schedule.RecipientAccount); err != nil {
				failedUser = &user
				return recordScheduleFailure(tx, &schedule, &execRecord, nextRunAt, maxFailures, err.Error(), &failureReason)
			}
			if stellarClient != nil {
				if err := stellarClient.ValidateAccount(ctx, schedule.RecipientAccount); err != nil {
					failedUser = &user
					return recordScheduleFailure(tx, &schedule, &execRecord, nextRunAt, maxFailures, fmt.Sprintf("recipient validation failed: %v", err), &failureReason)
				}
			}

			params := services.RemittanceCreateParams{
				SenderID:         schedule.UserID,
				SenderAccount:    schedule.SenderAccount,
				RecipientAccount: schedule.RecipientAccount,
				Amount:           schedule.Amount,
				AssetCode:        schedule.Currency,
				Notes:            schedule.Notes,
			}
			payment, _, err := services.CreateRemittanceInTx(tx, fees, params)
			if err != nil {
				failedUser = &user
				return recordScheduleFailure(tx, &schedule, &execRecord, nextRunAt, maxFailures, err.Error(), &failureReason)
			}

			execRecord.Status = "succeeded"
			execRecord.PaymentID = &payment.ID
			if err := tx.Save(&execRecord).Error; err != nil {
				return err
			}

			schedule.LastRunAt = &nowUTC
			schedule.NextRunAt = nextRunAt
			schedule.ConsecutiveFailures = 0
			schedule.LastFailureReason = ""
			if err := tx.Save(&schedule).Error; err != nil {
				return err
			}

			processed++
			return nil
		})

		if err != nil {
			logger.Log.WithField("schedule_id", scheduleID).WithField("error", err).Error("Failed processing recurring schedule")
			continue
		}

		if failureReason != "" && failedUser != nil && emailService != nil {
			if failedPayment == nil {
				failedPayment = &models.Payment{SenderID: failedUser.ID}
			}
			_ = emailService.SendPaymentFailedEmail(failedUser, failedPayment, failureReason)
		}
	}

	return processed, nil
}

func recordScheduleFailure(
	tx *gorm.DB,
	schedule *models.RecurringSchedule,
	execRecord *models.RecurringScheduleExecution,
	nextRunAt time.Time,
	maxFailures int,
	reason string,
	outReason *string,
) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	*outReason = reason
	execRecord.Status = "failed"
	execRecord.ErrorMessage = reason
	if err := tx.Save(execRecord).Error; err != nil {
		return err
	}

	schedule.ConsecutiveFailures++
	schedule.LastFailureReason = reason
	schedule.NextRunAt = nextRunAt
	if schedule.ConsecutiveFailures >= maxFailures {
		schedule.Status = models.RecurringStatusPaused
	}
	return tx.Save(schedule).Error
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}
