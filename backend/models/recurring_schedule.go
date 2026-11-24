package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

const (
	RecurringStatusActive    = "active"
	RecurringStatusPaused    = "paused"
	RecurringStatusCancelled = "cancelled"

	RecurringFreqDaily   = "daily"
	RecurringFreqWeekly  = "weekly"
	RecurringFreqMonthly = "monthly"
)

type RecurringSchedule struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
	UserID              uint           `gorm:"index;not null" json:"user_id"`
	SenderAccount       string         `gorm:"size:56;not null" json:"sender_account"`
	RecipientAccount    string         `gorm:"size:56;not null" json:"recipient_account"`
	Amount              int64          `gorm:"type:bigint;not null" json:"amount"`
	Currency            string         `gorm:"size:12;not null" json:"currency"`
	AssetIssuer         string         `gorm:"size:56" json:"asset_issuer"`
	Frequency           string         `gorm:"size:20;not null" json:"frequency"`
	Status              string         `gorm:"size:20;default:'active';index" json:"status"`
	NextRunAt           time.Time      `gorm:"index;not null" json:"next_run_at"`
	LastRunAt           *time.Time     `json:"last_run_at"`
	ConsecutiveFailures int            `gorm:"default:0" json:"consecutive_failures"`
	LastFailureReason   string         `gorm:"size:500" json:"last_failure_reason,omitempty"`
	Notes               string         `gorm:"size:500" json:"notes"`
}

func (RecurringSchedule) TableName() string {
	return "recurring_schedules"
}

// RecurringScheduleExecution records each claimed scheduled_for timestamp per schedule.
// The composite unique index guarantees at most one remittance per scheduled run.
type RecurringScheduleExecution struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	ScheduleID   uint      `gorm:"uniqueIndex:idx_schedule_execution;not null" json:"schedule_id"`
	ScheduledFor time.Time `gorm:"uniqueIndex:idx_schedule_execution;not null" json:"scheduled_for"`
	PaymentID    *uint     `json:"payment_id"`
	Status       string    `gorm:"size:20;not null" json:"status"` // succeeded, failed
	ErrorMessage string    `gorm:"size:500" json:"error_message,omitempty"`
}

func (RecurringScheduleExecution) TableName() string {
	return "recurring_schedule_executions"
}

func IsValidRecurringFrequency(freq string) bool {
	return freq == RecurringFreqDaily || freq == RecurringFreqWeekly || freq == RecurringFreqMonthly
}

// ComputeNextRunAt computes the next scheduled run in UTC from `from`.
// Monthly increments clamp the day of month to the last day of the target month
// (e.g. Jan 31 + 1 month -> Feb 28 or Feb 29 in a leap year).
func ComputeNextRunAt(from time.Time, frequency string) (time.Time, error) {
	t := from.UTC()
	switch frequency {
	case RecurringFreqDaily:
		return t.AddDate(0, 0, 1), nil
	case RecurringFreqWeekly:
		return t.AddDate(0, 0, 7), nil
	case RecurringFreqMonthly:
		year, month, day := t.Date()
		hour, min, sec := t.Clock()
		nsec := t.Nanosecond()

		targetMonth := month + 1
		targetYear := year
		if targetMonth > 12 {
			targetMonth = 1
			targetYear++
		}
		// Day 0 of the month after targetMonth gives the last day of targetMonth.
		daysInTarget := time.Date(targetYear, targetMonth+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if day > daysInTarget {
			day = daysInTarget
		}
		return time.Date(targetYear, targetMonth, day, hour, min, sec, nsec, time.UTC), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported frequency: %s", frequency)
	}
}

// NextFutureRunAt advances from `from` by `frequency` until the result is strictly after `now` (UTC).
// This prevents a burst of catch-up payments after downtime or when resuming a paused schedule.
func NextFutureRunAt(from, now time.Time, frequency string) (time.Time, error) {
	nowUTC := now.UTC()
	next, err := ComputeNextRunAt(from.UTC(), frequency)
	if err != nil {
		return time.Time{}, err
	}
	for !next.After(nowUTC) {
		next, err = ComputeNextRunAt(next, frequency)
		if err != nil {
			return time.Time{}, err
		}
	}
	return next, nil
}
