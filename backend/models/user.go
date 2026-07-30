package models

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID                    uint           `gorm:"primaryKey" json:"id"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
	Email                 string         `gorm:"uniqueIndex;size:255;not null" json:"email"`
	Name                  string         `gorm:"size:255;not null" json:"name"`
	StellarAddress        string         `gorm:"uniqueIndex;size:56;not null" json:"stellar_address"`
	PasswordHash          string         `gorm:"size:255;not null" json:"-"`
	Role                  string         `gorm:"size:20;default:'user'" json:"role"`
	Country               string         `gorm:"size:2" json:"country"`
	KYCStatus             string         `gorm:"size:20;default:'pending'" json:"kyc_status"`
	KYCVerifiedAt         *time.Time     `json:"kyc_verified_at"`
	IsActive              bool           `gorm:"index;default:true" json:"is_active"`
	DefaultCurrency       string         `gorm:"size:10;default:'USD'" json:"default_currency"`
	EmailNotifications    bool           `gorm:"default:true" json:"email_notifications"`
	ResetToken            string         `gorm:"size:255;index" json:"-"`
	ResetTokenExpiresAt   *time.Time     `json:"-"`
	FailedLoginAttempts   int            `gorm:"default:0" json:"-"`
	LockedUntil           *time.Time     `gorm:"index" json:"-"`
	LastFailedLoginAt     *time.Time     `json:"-"`
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
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, c := range password {
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsDigit(c):
			hasDigit = true
		case unicode.IsPunct(c) || unicode.IsSymbol(c):
			hasSpecial = true
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
	if !hasSpecial {
		return errors.New("password must contain at least one special character")
	}

	if err := checkCommonPassword(password); err != nil {
		return err
	}

	return nil
}

// checkCommonPassword uses HaveIBeenPwned API (k-Anonymity) to check if the password is known
func checkCommonPassword(password string) error {
	hasher := sha1.New()
	hasher.Write([]byte(password))
	hash := strings.ToUpper(hex.EncodeToString(hasher.Sum(nil)))

	prefix := hash[:5]
	suffix := hash[5:]

	url := fmt.Sprintf("https://api.pwnedpasswords.com/range/%s", prefix)
	resp, err := http.Get(url)
	if err != nil {
		// Fail open if the external API is down so users can still register
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			bodyString := string(bodyBytes)
			lines := strings.Split(bodyString, "\n")
			for _, line := range lines {
				parts := strings.Split(strings.TrimSpace(line), ":")
				if len(parts) >= 1 && parts[0] == suffix {
					return errors.New("password is too common or has been compromised in a data breach")
				}
			}
		}
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

// IsAccountLocked checks if the user account is currently locked
func (u *User) IsAccountLocked() bool {
	if u.LockedUntil == nil {
		return false
	}
	return time.Now().Before(*u.LockedUntil)
}

// RecordFailedLogin records a failed login attempt and locks account if threshold reached
func (u *User) RecordFailedLogin(db *gorm.DB) error {
	now := time.Now()
	
	// Reset counter if last failed login was more than 15 minutes ago
	if u.LastFailedLoginAt != nil && now.Sub(*u.LastFailedLoginAt) > 15*time.Minute {
		u.FailedLoginAttempts = 0
	}
	
	u.FailedLoginAttempts++
	u.LastFailedLoginAt = &now
	
	// Lock account after 5 failed attempts
	if u.FailedLoginAttempts >= 5 {
		lockUntil := now.Add(30 * time.Minute)
		u.LockedUntil = &lockUntil
	}
	
	return db.Save(u).Error
}

// ResetFailedLoginAttempts clears failed login attempts after successful login
func (u *User) ResetFailedLoginAttempts(db *gorm.DB) error {
	u.FailedLoginAttempts = 0
	u.LastFailedLoginAt = nil
	u.LockedUntil = nil
	return db.Save(u).Error
}
