package models

import (
	"time"

	"gorm.io/gorm"
)

type Payment struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	SenderID        uint           `gorm:"not null" json:"sender_id"`
	RecipientID     uint           `gorm:"not null" json:"recipient_id"`
	Amount          float64        `gorm:"not null" json:"amount"`
	Currency        string         `gorm:"size:10;not null" json:"currency"`
	TargetCurrency  string         `gorm:"size:10" json:"target_currency"`
	ConvertedAmount float64        `json:"converted_amount"`
	Status          string         `gorm:"size:20;default:'pending'" json:"status"` // pending, processing, completed, failed
	TxHash          string         `gorm:"size:255" json:"tx_hash"`
	ContractID      string         `gorm:"size:255" json:"contract_id"`
	EscrowID        string         `gorm:"size:255" json:"escrow_id"`
	Fee             float64        `gorm:"default:0" json:"fee"`
	Notes           string         `gorm:"type:text" json:"notes"`
}

// TableName overrides the table name
func (Payment) TableName() string {
	return "payments"
}
