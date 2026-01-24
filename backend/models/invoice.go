package models

import (
	"time"

	"gorm.io/gorm"
)

type Invoice struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	PaymentID   uint           `gorm:"not null" json:"payment_id"`
	Payment     Payment        `gorm:"foreignKey:PaymentID" json:"payment,omitempty"`
	InvoiceNo   string         `gorm:"uniqueIndex;size:50;not null" json:"invoice_no"`
	IssuerID    uint           `gorm:"not null" json:"issuer_id"`
	RecipientID uint           `gorm:"not null" json:"recipient_id"`
	Amount      float64        `gorm:"not null" json:"amount"`
	Currency    string         `gorm:"size:10;not null" json:"currency"`
	DueDate     *time.Time     `json:"due_date"`
	Status      string         `gorm:"size:20;default:'unpaid'" json:"status"` // unpaid, paid, overdue, cancelled
	Description string         `gorm:"type:text" json:"description"`
	PdfURL      string         `gorm:"size:500" json:"pdf_url"`
}

// TableName overrides the table name
func (Invoice) TableName() string {
	return "invoices"
}
