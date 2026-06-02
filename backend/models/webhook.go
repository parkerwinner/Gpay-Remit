package models

import (
	"time"

	"gorm.io/gorm"
)

type Webhook struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	UserID      uint           `gorm:"index;not null" json:"user_id"`
	URL         string         `gorm:"size:500;not null" json:"url"`
	Secret      string         `gorm:"size:255;not null" json:"-"` // HMAC secret for signature verification
	Events      string         `gorm:"type:text;not null" json:"events"` // Comma-separated list of events
	IsActive    bool           `gorm:"index;default:true" json:"is_active"`
	Description string         `gorm:"size:255" json:"description"`
}

type WebhookDelivery struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	WebhookID     uint           `gorm:"index;not null" json:"webhook_id"`
	Event         string         `gorm:"size:100;not null" json:"event"`
	Payload       string         `gorm:"type:text;not null" json:"payload"`
	Status        string         `gorm:"index;size:20;default:'pending'" json:"status"` // pending, success, failed
	ResponseCode  int            `json:"response_code"`
	ResponseBody  string         `gorm:"type:text" json:"response_body"`
	ErrorMessage  string         `gorm:"type:text" json:"error_message"`
	AttemptCount  int            `gorm:"default:0" json:"attempt_count"`
	NextRetryAt   *time.Time     `json:"next_retry_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
}

// TableName overrides the table name
func (Webhook) TableName() string {
	return "webhooks"
}

// TableName overrides the table name
func (WebhookDelivery) TableName() string {
	return "webhook_deliveries"
}
