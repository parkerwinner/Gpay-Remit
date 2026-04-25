package models

import (
	"time"

	"gorm.io/gorm"
)

// IdempotencyRecord stores idempotency key information for request deduplication
type IdempotencyRecord struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	IdempotencyKey   string         `gorm:"size:256;not null;index" json:"idempotency_key"`
	RequestHash      string         `gorm:"size:64;not null" json:"request_hash"`
	RequestMethod    string         `gorm:"size:10;not null" json:"request_method"`
	RequestPath      string         `gorm:"size:512;not null" json:"request_path"`
	Status           string         `gorm:"size:20;not null;default:'processing'" json:"status"` // processing, completed, failed
	ResponseStatus   int            `gorm:"default:0" json:"response_status"`
	ResponseBody     string         `gorm:"type:text" json:"response_body"`
	CreatedAtUnix    int64          `gorm:"not null" json:"created_at_unix"`
	ExpiresAt        time.Time      `gorm:"index" json:"expires_at"`
	CompletedAt      *time.Time     `json:"completed_at"`
	RequestBody      string         `gorm:"type:text" json:"request_body"` // Optional: store request body for debugging
	UserID           uint           `gorm:"index" json:"user_id"`          // Optional: associate with user
	IPAddress        string         `gorm:"size:45" json:"ip_address"`     // Optional: store IP for audit
}

// TableName overrides the table name
func (IdempotencyRecord) TableName() string {
	return "idempotency_records"
}

// BeforeCreate sets default values before creating a record
func (i *IdempotencyRecord) BeforeCreate(tx *gorm.DB) error {
	i.CreatedAtUnix = time.Now().Unix()
	if i.ExpiresAt.IsZero() {
		i.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	return nil
}

// IsExpired checks if the idempotency record has expired
func (i *IdempotencyRecord) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

// IsCompleted checks if the request has been processed
func (i *IdempotencyRecord) IsCompleted() bool {
	return i.Status == "completed"
}

// IsProcessing checks if the request is still being processed
func (i *IdempotencyRecord) IsProcessing() bool {
	return i.Status == "processing"
}

// MarkCompleted marks the record as completed with the response
func (i *IdempotencyRecord) MarkCompleted(statusCode int, responseBody string) {
	i.Status = "completed"
	i.ResponseStatus = statusCode
	i.ResponseBody = responseBody
	now := time.Now()
	i.CompletedAt = &now
}

// MarkFailed marks the record as failed
func (i *IdempotencyRecord) MarkFailed(statusCode int, errorMessage string) {
	i.Status = "failed"
	i.ResponseStatus = statusCode
	i.ResponseBody = errorMessage
	now := time.Now()
	i.CompletedAt = &now
}