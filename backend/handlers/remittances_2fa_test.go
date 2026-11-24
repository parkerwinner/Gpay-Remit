package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupLargeTransfer2FATest(t *testing.T) (*gorm.DB, *gin.Engine, models.User, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Payment{}, &models.PaymentTag{}))

	cfg := &config.Config{
		JWTSecret:            "test-secret-key-at-least-32-characters-long",
		Requires2FAThreshold: 100000, // $1,000.00
	}

	user := models.User{
		Email:          "sender@example.com",
		Name:           "Sender",
		StellarAddress: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
		PasswordHash:   "hash",
		MFAEnabled:     true,
	}
	secret, _, err := user.GenerateTOTPSecret()
	require.NoError(t, err)
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set("userID", user.ID)
		c.Next()
	})

	emailSvc := services.NewEmailService("", "", "", "", "", false)
	authHandler := NewAuthHandler(db, cfg, emailSvc)
	remitHandler := NewRemittanceHandler(db, cfg)

	r.POST("/auth/mfa/step-up", authHandler.StepUpMFA)
	r.POST("/remittances", remitHandler.SendRemittance)

	return db, r, user, secret
}

func TestLargeTransfer_BelowThresholdSucceedsWithout2FA(t *testing.T) {
	_, r, user, _ := setupLargeTransfer2FATest(t)

	body, _ := json.Marshal(map[string]interface{}{
		"sender_id":       user.ID,
		"recipient_id":    2,
		"amount":          50000, // $500 < $1,000 threshold
		"currency":        "USD",
		"target_currency": "EUR",
	})
	req := httptest.NewRequest(http.MethodPost, "/remittances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestLargeTransfer_AboveThresholdRequires2FAStepUp(t *testing.T) {
	_, r, user, secret := setupLargeTransfer2FATest(t)

	payload := map[string]interface{}{
		"sender_id":       user.ID,
		"recipient_id":    2,
		"amount":          250000, // $2,500 > $1,000 threshold
		"currency":        "USD",
		"target_currency": "EUR",
	}
	body, _ := json.Marshal(payload)

	// 1. Without step-up token -> 403 STEP_UP_2FA_REQUIRED
	req := httptest.NewRequest(http.MethodPost, "/remittances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "STEP_UP_2FA_REQUIRED")

	// 2. Obtain step-up token via /auth/mfa/step-up
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	stepUpBody, _ := json.Marshal(map[string]string{"code": code})
	stepUpReq := httptest.NewRequest(http.MethodPost, "/auth/mfa/step-up", bytes.NewReader(stepUpBody))
	stepUpReq.Header.Set("Content-Type", "application/json")
	stepUpW := httptest.NewRecorder()
	r.ServeHTTP(stepUpW, stepUpReq)

	assert.Equal(t, http.StatusOK, stepUpW.Code)
	var stepUpResp struct {
		StepUpToken string `json:"step_up_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	require.NoError(t, json.Unmarshal(stepUpW.Body.Bytes(), &stepUpResp))
	assert.NotEmpty(t, stepUpResp.StepUpToken)

	// 3. Retry transfer with X-Step-Up-Token -> 201 Created
	req2 := httptest.NewRequest(http.MethodPost, "/remittances", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Step-Up-Token", stepUpResp.StepUpToken)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusCreated, w2.Code)
}

func TestLargeTransfer_RejectsWhenMFANotEnabled(t *testing.T) {
	db, r, user, _ := setupLargeTransfer2FATest(t)
	require.NoError(t, db.Model(&user).Update("mfa_enabled", false).Error)

	body, _ := json.Marshal(map[string]interface{}{
		"sender_id":       user.ID,
		"recipient_id":    2,
		"amount":          250000,
		"currency":        "USD",
		"target_currency": "EUR",
	})
	req := httptest.NewRequest(http.MethodPost, "/remittances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "MFA_SETUP_REQUIRED")
}
