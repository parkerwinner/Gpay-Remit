package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// ExportSchedule tracks automated scheduled transaction exports.
type ExportSchedule struct {
	gorm.Model
	UserID      uint       `gorm:"index;not null" json:"user_id"`
	Frequency   string     `gorm:"size:20;not null" json:"frequency"`   // daily, weekly, monthly
	Format      string     `gorm:"size:10;not null" json:"format"`      // csv, pdf
	Destination string     `gorm:"size:20;not null" json:"destination"` // email, webhook
	WebhookURL  string     `gorm:"size:500" json:"webhook_url,omitempty"`
	IsActive    bool       `gorm:"index;default:true" json:"is_active"`
	LastRunAt   *time.Time `json:"last_run_at,omitempty"`
	NextRunAt   time.Time  `gorm:"index;not null" json:"next_run_at"`
	LastStatus  string     `gorm:"size:20" json:"last_status,omitempty"`
	LastError   string     `gorm:"type:text" json:"last_error,omitempty"`
}

// TableName overrides the table name.
func (ExportSchedule) TableName() string {
	return "export_schedules"
}

// IsValidExportFrequency reports whether frequency is supported.
func IsValidExportFrequency(frequency string) bool {
	switch strings.ToLower(strings.TrimSpace(frequency)) {
	case "daily", "weekly", "monthly":
		return true
	default:
		return false
	}
}

// IsValidExportFormat reports whether format is supported.
func IsValidExportFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "csv", "pdf":
		return true
	default:
		return false
	}
}

// IsValidExportDestination reports whether destination is supported.
func IsValidExportDestination(destination string) bool {
	switch strings.ToLower(strings.TrimSpace(destination)) {
	case "email", "webhook":
		return true
	default:
		return false
	}
}

// ComputeNextExportRun returns the next scheduled run time after from.
func ComputeNextExportRun(from time.Time, frequency string) time.Time {
	base := from.UTC()
	switch strings.ToLower(strings.TrimSpace(frequency)) {
	case "daily":
		return base.AddDate(0, 0, 1)
	case "weekly":
		return base.AddDate(0, 0, 7)
	case "monthly":
		return base.AddDate(0, 1, 0)
	default:
		return base.AddDate(0, 0, 1)
	}
}
