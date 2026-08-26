package models

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ContactVerificationStatus string

const (
	ContactVerificationPending  ContactVerificationStatus = "pending"
	ContactVerificationVerified ContactVerificationStatus = "verified"
	ContactVerificationFailed   ContactVerificationStatus = "failed"
)

type Contact struct {
	ID                 uint                      `gorm:"primaryKey" json:"id"`
	CreatedAt          time.Time                 `json:"created_at"`
	UpdatedAt          time.Time                 `json:"updated_at"`
	DeletedAt          gorm.DeletedAt            `gorm:"index" json:"-"`
	UserID             uint                      `gorm:"index;not null" json:"user_id"`
	Nickname           string                    `gorm:"size:255;not null" json:"nickname"`
	StellarAddress     string                    `gorm:"size:56;not null" json:"stellar_address"`
	Currency           string                    `gorm:"size:12;default:'USDC'" json:"currency"`
	Email              string                    `gorm:"size:255" json:"email,omitempty"`
	Notes              string                    `gorm:"size:500" json:"notes,omitempty"`
	VerificationStatus ContactVerificationStatus `gorm:"size:20;default:'pending'" json:"verification_status"`
	IsVerified         bool                      `gorm:"default:false" json:"is_verified"`
}

func (Contact) TableName() string {
	return "contacts"
}

func (c *Contact) Validate() error {
	if strings.TrimSpace(c.Nickname) == "" {
		return errors.New("nickname is required")
	}
	if strings.TrimSpace(c.StellarAddress) == "" {
		return errors.New("stellar address is required")
	}
	if len(c.StellarAddress) != 56 || !strings.HasPrefix(c.StellarAddress, "G") {
		return errors.New("invalid stellar address format")
	}
	return nil
}
