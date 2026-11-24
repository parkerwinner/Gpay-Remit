package models

import (
	"time"

	"gorm.io/gorm"
)

// EmailCategory classifies outgoing emails so user preferences can be enforced
// per category while ensuring security alerts can never be disabled.
type EmailCategory string

const (
	EmailCategoryTransaction EmailCategory = "transaction_confirmations"
	EmailCategoryMarketing   EmailCategory = "marketing"
	EmailCategorySecurity    EmailCategory = "security_alerts"
)

// EmailPreferences stores per-user email notification opt-in/opt-out settings.
// SecurityAlerts is always enforced as true and cannot be disabled.
type EmailPreferences struct {
	ID                       uint      `gorm:"primaryKey" json:"id"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
	UserID                   uint      `gorm:"uniqueIndex;not null" json:"user_id"`
	TransactionConfirmations bool      `gorm:"default:true;not null" json:"transaction_confirmations"`
	Marketing                bool      `gorm:"default:false;not null" json:"marketing"`
	SecurityAlerts           bool      `gorm:"default:true;not null" json:"security_alerts"`
}

// TableName overrides the table name.
func (EmailPreferences) TableName() string {
	return "email_preferences"
}

// DefaultEmailPreferences returns the default preferences for a user.
func DefaultEmailPreferences(userID uint) EmailPreferences {
	return EmailPreferences{
		UserID:                   userID,
		TransactionConfirmations: true,
		Marketing:                false,
		SecurityAlerts:           true,
	}
}

// BeforeSave ensures SecurityAlerts can never be persisted as false.
func (p *EmailPreferences) BeforeSave(tx *gorm.DB) error {
	p.SecurityAlerts = true
	return nil
}

// Allows reports whether the given email category is permitted by these preferences.
// Security alerts are always allowed regardless of stored fields.
func (p *EmailPreferences) Allows(category EmailCategory) bool {
	if category == EmailCategorySecurity {
		return true
	}
	if p == nil {
		return category == EmailCategoryTransaction
	}
	switch category {
	case EmailCategoryTransaction:
		return p.TransactionConfirmations
	case EmailCategoryMarketing:
		return p.Marketing
	default:
		return true
	}
}
