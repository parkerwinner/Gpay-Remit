package models

import (
	"errors"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
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
	Role            string         `gorm:"size:20;default:'user'" json:"role"`
	Country         string         `gorm:"size:2" json:"country"`
	KYCStatus       string         `gorm:"size:20;default:'pending'" json:"kyc_status"`
	KYCVerifiedAt   *time.Time     `json:"kyc_verified_at"`
	IsActive        bool           `gorm:"default:true" json:"is_active"`
	DefaultCurrency string         `gorm:"size:10;default:'USD'" json:"default_currency"`
}

// TableName overrides the table name.
func (User) TableName() string {
	return "users"
}

// ValidatePasswordStrength enforces minimum password requirements before hashing.
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}
	var hasUpper, hasLower, hasDigit bool
	for _, c := range password {
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsDigit(c):
			hasDigit = true
		}
	}
	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return errors.New("password must contain at least one digit")
	}
	return nil
}

// HashPassword validates password strength then hashes it using bcrypt with cost 12.
func HashPassword(password string) (string, error) {
	if err := ValidatePasswordStrength(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePassword reports whether the plaintext password matches the stored bcrypt hash.
func ComparePassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
