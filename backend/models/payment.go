package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type Payment struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	SenderID        uint           `gorm:"index;not null" json:"sender_id"`
	SenderAccount   string         `gorm:"size:56" json:"sender_account"`
	RecipientID     uint           `gorm:"index;not null" json:"recipient_id"`
	RecipientAccount string        `gorm:"size:56" json:"recipient_account"`
	Amount          int64          `gorm:"type:bigint;not null" json:"amount"`
	Currency        string         `gorm:"size:10;not null" json:"currency"`
	TargetCurrency  string         `gorm:"size:10" json:"target_currency"`
	ConvertedAmount int64          `gorm:"type:bigint" json:"converted_amount"`
	Status          string         `gorm:"index;size:20;default:'pending'" json:"status"` // pending, processing, completed, failed
	TxHash          string         `gorm:"index;size:255" json:"tx_hash"`
	ContractID      string         `gorm:"size:255" json:"contract_id"`
	EscrowID        string         `gorm:"index;size:255" json:"escrow_id"`
	// Fee is the total of all fee components.
	Fee           int64 `gorm:"type:bigint;default:0" json:"fee"`
	PlatformFee   int64 `gorm:"type:bigint;default:0" json:"platform_fee"`
	ForexFee      int64 `gorm:"type:bigint;default:0" json:"forex_fee"`
	ComplianceFee int64 `gorm:"type:bigint;default:0" json:"compliance_fee"`
	NetworkFee    int64 `gorm:"type:bigint;default:0" json:"network_fee"`
	Conditions      string         `gorm:"type:text" json:"conditions"` // JSON blob of conditions
	Notes           string         `gorm:"type:text" json:"notes"`
	Tags            []string       `gorm:"-" json:"tags"`
	SearchVector    string         `gorm:"type:tsvector" json:"-"`
}

// AfterFind ensures Tags is always serialized as an empty JSON array rather than null.
func (p *Payment) AfterFind(tx *gorm.DB) error {
	if p.Tags == nil {
		p.Tags = []string{}
	}
	return nil
}

// TableName overrides the table name
func (Payment) TableName() string {
	return "payments"
}

// SearchableText returns a concatenated text used for searching/highlighting
func (p *Payment) SearchableText() string {
	return fmt.Sprintf("%v %s %s %s", p.Amount, p.Currency, p.Status, p.Notes)
}
