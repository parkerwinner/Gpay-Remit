package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/models"
)

func TestNewEmailService(t *testing.T) {
	service := NewEmailService("smtp.gmail.com", "465", "test@example.com", "password", "noreply@example.com", true)
	
	assert.NotNil(t, service)
	assert.Equal(t, "smtp.gmail.com", service.smtpHost)
	assert.Equal(t, "465", service.smtpPort)
	assert.Equal(t, "test@example.com", service.smtpUser)
	assert.Equal(t, true, service.enabled)
}

func TestSendPaymentCompletedEmail_DisabledService(t *testing.T) {
	service := NewEmailService("smtp.gmail.com", "465", "test@example.com", "password", "noreply@example.com", false)
	
	user := &models.User{
		Email:              "user@example.com",
		Name:               "Test User",
		EmailNotifications: true,
	}
	
	payment := &models.Payment{
		ID:               1,
		Amount:           100.50,
		Currency:         "USD",
		RecipientAccount: "RECIPIENT123",
		Fee:              2.5,
		Status:           "completed",
		CreatedAt:        time.Now(),
	}
	
	// Should not error when service is disabled
	err := service.SendPaymentCompletedEmail(user, payment)
	assert.NoError(t, err)
}

func TestSendPaymentCompletedEmail_UserOptedOut(t *testing.T) {
	service := NewEmailService("smtp.gmail.com", "465", "test@example.com", "password", "noreply@example.com", true)
	
	user := &models.User{
		Email:              "user@example.com",
		Name:               "Test User",
		EmailNotifications: false, // User opted out
	}
	
	payment := &models.Payment{
		ID:               1,
		Amount:           100.50,
		Currency:         "USD",
		RecipientAccount: "RECIPIENT123",
		Fee:              2.5,
		Status:           "completed",
		CreatedAt:        time.Now(),
	}
	
	// Should not error when user opted out
	err := service.SendPaymentCompletedEmail(user, payment)
	assert.NoError(t, err)
}

func TestSendEscrowExpirationWarningEmail_UserOptedOut(t *testing.T) {
	service := NewEmailService("smtp.gmail.com", "465", "test@example.com", "password", "noreply@example.com", true)
	
	user := &models.User{
		Email:              "user@example.com",
		Name:               "Test User",
		EmailNotifications: false,
	}
	
	payment := &models.Payment{
		ID:               1,
		Amount:           100.50,
		Currency:         "USD",
		RecipientAccount: "RECIPIENT123",
		EscrowID:         "ESCROW123",
		CreatedAt:        time.Now(),
	}
	
	err := service.SendEscrowExpirationWarningEmail(user, payment, 24)
	assert.NoError(t, err)
}

func TestSendPaymentFailedEmail_UserOptedOut(t *testing.T) {
	service := NewEmailService("smtp.gmail.com", "465", "test@example.com", "password", "noreply@example.com", true)
	
	user := &models.User{
		Email:              "user@example.com",
		Name:               "Test User",
		EmailNotifications: false,
	}
	
	payment := &models.Payment{
		ID:               1,
		Amount:           100.50,
		Currency:         "USD",
		RecipientAccount: "RECIPIENT123",
		CreatedAt:        time.Now(),
	}
	
	err := service.SendPaymentFailedEmail(user, payment, "Insufficient funds")
	assert.NoError(t, err)
}

func TestEmailTemplateGeneration(t *testing.T) {
	service := NewEmailService("smtp.gmail.com", "465", "test@example.com", "password", "noreply@example.com", false)
	
	user := &models.User{
		Email:              "user@example.com",
		Name:               "John Doe",
		EmailNotifications: true,
	}
	
	payment := &models.Payment{
		ID:               123,
		Amount:           250.75,
		Currency:         "USD",
		RecipientAccount: "GABCDEFG123456",
		Fee:              5.25,
		Status:           "completed",
		EscrowID:         "ESC789",
		CreatedAt:        time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}
	
	// Test that templates can be generated without errors
	err := service.SendPaymentCompletedEmail(user, payment)
	assert.NoError(t, err)
	
	err = service.SendEscrowExpirationWarningEmail(user, payment, 48)
	assert.NoError(t, err)
	
	err = service.SendPaymentFailedEmail(user, payment, "Network error")
	assert.NoError(t, err)
}
