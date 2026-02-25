package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&models.Payment{}, &models.User{})
	return db
}

type MockStellarClient struct {
	ValidateAccountFunc func(accountID string) error
	BuildEscrowTxFunc   func(sender, recipient, assetCode, issuer, amount string) (string, error)
	SubmitPaymentFunc   func(sourceSecret, destination, assetCode, issuer, amount string) (string, error)
}

func (m *MockStellarClient) ValidateAccount(accountID string) error {
	return m.ValidateAccountFunc(accountID)
}

func (m *MockStellarClient) BuildEscrowTx(sender, recipient, assetCode, issuer, amount string) (string, error) {
	return m.BuildEscrowTxFunc(sender, recipient, assetCode, issuer, amount)
}

func (m *MockStellarClient) SubmitPayment(sourceSecret, destination, assetCode, issuer, amount string) (string, error) {
	return m.SubmitPaymentFunc(sourceSecret, destination, assetCode, issuer, amount)
}

func TestCreateRemittance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	mockStellar := &MockStellarClient{
		ValidateAccountFunc: func(accountID string) error { return nil },
		BuildEscrowTxFunc:   func(sender, recipient, assetCode, issuer, amount string) (string, error) { return "base64_xdr", nil },
	}
	handler := &RemittanceHandler{
		db:            db,
		config:        &config.Config{},
		stellarClient: mockStellar,
	}

	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	})
	router.POST("/remittances/create", handler.CreateRemittance)

	t.Run("Valid Request", func(t *testing.T) {
		reqBody := CreateRemittanceRequest{
			SenderAccount:   "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			RecipientAccount: "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			Amount:          100.50,
			AssetCode:       "USDC",
			Conditions:      map[string]interface{}{"note": "test"},
		}
		body, _ := json.Marshal(reqBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/create", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), "base64_xdr")

		var payment models.Payment
		db.First(&payment)
		assert.Equal(t, 100.50, payment.Amount)
		assert.Equal(t, "USDC", payment.Currency)
	})

	t.Run("Invalid Amount", func(t *testing.T) {
		reqBody := CreateRemittanceRequest{
			SenderAccount:   "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			RecipientAccount: "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			Amount:          -10,
			AssetCode:       "USDC",
		}
		body, _ := json.Marshal(reqBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/create", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
