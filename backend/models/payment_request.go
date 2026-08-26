package models

import (
	"time"

	"gorm.io/gorm"
)

type PaymentRequest struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	RequesterID     uint           `gorm:"index;not null" json:"requester_id"`
	TargetUserID    uint           `gorm:"index;not null" json:"target_user_id"`
	Amount          float64        `gorm:"not null" json:"amount"`
	Currency        string         `gorm:"size:10;not null" json:"currency"`
	AssetCode       string         `gorm:"size:12;not null" json:"asset_code"`
	AssetIssuer     string         `gorm:"size:56" json:"asset_issuer"`
	Description     string         `gorm:"type:text" json:"description"`
	Reference       string         `gorm:"size:255" json:"reference"`
	Status          string         `gorm:"index;size:20;default:'pending'" json:"status"` // pending, accepted, rejected, expired, paid
	ExpiresAt       *time.Time     `gorm:"index" json:"expires_at"`
	AcceptedAt      *time.Time     `json:"accepted_at"`
	RejectedAt      *time.Time     `json:"rejected_at"`
	PaidAt          *time.Time     `json:"paid_at"`
	RejectionReason string         `gorm:"type:text" json:"rejection_reason"`
	Notes           string         `gorm:"type:text" json:"notes"`
	PaymentID       *uint          `gorm:"index" json:"payment_id"`
	Payment         *Payment       `gorm:"foreignKey:PaymentID" json:"payment,omitempty"`
}

func (PaymentRequest) TableName() string {
	return "payment_requests"
}

func CreatePaymentRequest(db *gorm.DB, req *PaymentRequest) error {
	return db.Create(req).Error
}

func GetPaymentRequest(db *gorm.DB, id uint) (*PaymentRequest, error) {
	var pr PaymentRequest
	err := db.First(&pr, id).Error
	return &pr, err
}

func GetPaymentRequestByIDAndUser(db *gorm.DB, id uint, userID uint) (*PaymentRequest, error) {
	var pr PaymentRequest
	err := db.Where("(requester_id = ? OR target_user_id = ?) AND id = ?", userID, userID, id).First(&pr).Error
	return &pr, err
}

func ListPaymentRequestsForUser(db *gorm.DB, userID uint, status string) ([]PaymentRequest, error) {
	var requests []PaymentRequest
	query := db.Where("requester_id = ? OR target_user_id = ?", userID, userID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Order("created_at DESC").Find(&requests).Error
	return requests, err
}

func UpdatePaymentRequest(db *gorm.DB, pr *PaymentRequest) error {
	return db.Save(pr).Error
}

func ExpireStalePaymentRequests(db *gorm.DB) (int64, error) {
	result := db.Model(&PaymentRequest{}).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at < ?", "pending", time.Now()).
		Update("status", "expired")
	return result.RowsAffected, result.Error
}
