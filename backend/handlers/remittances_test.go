package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/models"
	"github.com/yourusername/gpay-remit/services"
	"github.com/stellar/go/txnbuild"
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
	BuildPaymentTxFunc  func(sourceAccount txnbuild.Account, destination string, assetCode string, issuer string, amount string) (*txnbuild.Transaction, error)
	SignTxFunc          func(envelopeXDR string, secretKey string) (string, error)
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

func (m *MockStellarClient) BuildPaymentTx(sourceAccount txnbuild.Account, destination string, assetCode string, issuer string, amount string) (*txnbuild.Transaction, error) {
	return m.BuildPaymentTxFunc(sourceAccount, destination, assetCode, issuer, amount)
}

func (m *MockStellarClient) SignTx(envelopeXDR string, secretKey string) (string, error) {
	return m.SignTxFunc(envelopeXDR, secretKey)
}


func TestCreateRemittance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	mockStellar := &MockStellarClient{
		ValidateAccountFunc: func(accountID string) error { return nil },
		BuildEscrowTxFunc:   func(sender, recipient, assetCode, issuer, amount string) (string, error) { return "base64_xdr", nil },
	}
	testCfg := &config.Config{}
	handler := &RemittanceHandler{
		db:            db,
		config:        testCfg,
		stellarClient: mockStellar,
		fees:          services.NewFeeService(testCfg),
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
			SenderAccount:    "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			RecipientAccount: "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			Amount:           -10,
			AssetCode:        "USDC",
		}
		body, _ := json.Marshal(reqBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/create", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Zero Amount", func(t *testing.T) {
		reqBody := CreateRemittanceRequest{
			SenderAccount:    "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			RecipientAccount: "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			Amount:           0,
			AssetCode:        "USDC",
		}
		body, _ := json.Marshal(reqBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/create", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Missing Asset Code", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"sender_account":    "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			"recipient_account": "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			"amount":            100,
		}
		body, _ := json.Marshal(reqBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/create", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Stellar Client Failure", func(t *testing.T) {
		failCfg := &config.Config{}
		failHandler := &RemittanceHandler{
			db:     db,
			config: failCfg,
			fees:   services.NewFeeService(failCfg),
			stellarClient: &MockStellarClient{
				ValidateAccountFunc: func(accountID string) error { return nil },
				BuildEscrowTxFunc: func(sender, recipient, assetCode, issuer, amount string) (string, error) {
					return "", assert.AnError
				},
			},
		}
		failRouter := gin.New()
		failRouter.Use(func(c *gin.Context) {
			c.Set("userID", uint(1))
			c.Next()
		})
		failRouter.POST("/remittances/create", failHandler.CreateRemittance)

		reqBody := CreateRemittanceRequest{
			SenderAccount:    "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			RecipientAccount: "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			Amount:           50,
			AssetCode:        "USDC",
		}
		body, _ := json.Marshal(reqBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/create", bytes.NewBuffer(body))
		failRouter.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Large Amount", func(t *testing.T) {
		reqBody := CreateRemittanceRequest{
			SenderAccount:    "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			RecipientAccount: "GCO7V6V6VZ5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X6Z5X",
			Amount:           999999999.99,
			AssetCode:        "USDC",
		}
		body, _ := json.Marshal(reqBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/create", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})
}

// Test pagination overflow fix (#198)
func TestPaginationOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	
	// Create some test payments
	for i := 0; i < 5; i++ {
		payment := models.Payment{
			SenderID:    1,
			RecipientID: 2,
			Amount:      float64(100 + i),
			Currency:    "USD",
			Status:      "pending",
		}
		db.Create(&payment)
	}
	
	mockStellar := &MockStellarClient{}
	testCfg := &config.Config{}
	handler := &RemittanceHandler{
		db:            db,
		config:        testCfg,
		stellarClient: mockStellar,
		fees:          services.NewFeeService(testCfg),
	}

	router := gin.Default()
	router.GET("/remittances", handler.ListRemittances)

	t.Run("Valid MaxPage", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/remittances?page=10000&page_size=20", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Exceeds MaxPage", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/remittances?page=10001&page_size=20", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "cannot exceed 10000")
	})

	t.Run("Zero Page", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/remittances?page=0&page_size=20", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code) // Should default to page 1
	})

	t.Run("Negative Page", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/remittances?page=-1&page_size=20", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code) // Should default to page 1
	})
}

// Test cursor-based pagination (#198)
func TestCursorPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	
	// Create test payments with known timestamps
	now := time.Now()
	for i := 0; i < 5; i++ {
		payment := models.Payment{
			SenderID:    1,
			RecipientID: 2,
			Amount:      float64(100 + i),
			Currency:    "USD",
			Status:      "pending",
			CreatedAt:   now.Add(time.Duration(i) * time.Hour), // Different timestamps
		}
		db.Create(&payment)
	}
	
	mockStellar := &MockStellarClient{}
	testCfg := &config.Config{}
	handler := &RemittanceHandler{
		db:            db,
		config:        testCfg,
		stellarClient: mockStellar,
		fees:          services.NewFeeService(testCfg),
	}

	router := gin.Default()
	router.GET("/remittances", handler.ListRemittances)

	t.Run("Valid Cursor", func(t *testing.T) {
		// First request with limit
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/remittances?limit=2", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response ListRemittancesResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(response.Data))
		assert.True(t, response.HasMore)
		assert.NotEmpty(t, response.NextCursor)
	})

	t.Run("Invalid Cursor", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/remittances?cursor=invalid_base64&limit=2", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid cursor format")
	})

	t.Run("Empty Result Set", func(t *testing.T) {
		// Clear all payments
		db.Where("1 = 1").Delete(&models.Payment{})
		
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/remittances?limit=2", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response ListRemittancesResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(response.Data))
		assert.False(t, response.HasMore)
		assert.Empty(t, response.NextCursor)
	})
}
