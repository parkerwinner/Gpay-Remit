package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	Email           string         `gorm:"uniqueIndex;size:255;not null" json:"email"`
	Name            string         `gorm:"size:255;not null" json:"name"`
	StellarAddress  string         `gorm:"uniqueIndex;size:56;not null" json:"stellar_address"`
	PasswordHash    string         `gorm:"size:255;not null" json:"-"`
	Role            string         `gorm:"size:20;default:'user'" json:"role"` // admin, user
	Country         string         `gorm:"size:2" json:"country"` // ISO country code
	KYCStatus       string         `gorm:"size:20;default:'pending'" json:"kyc_status"` // pending, verified, rejected
	KYCVerifiedAt   *time.Time     `json:"kyc_verified_at"`
	IsActive        bool           `gorm:"default:true" json:"is_active"`
	DefaultCurrency string         `gorm:"size:10;default:'USD'" json:"default_currency"`
}

// TableName overrides the table name
func (User) TableName() string {
	return "users"
}
