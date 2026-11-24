package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	testSenderAddr     = "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF"
	testOtherAddr      = "GBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	testRecipientAddr1 = "GCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
	testRecipientAddr2 = "GDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"
)

func setupBatchRouter(db *gorm.DB, cfg *config.Config, stellar *MockStellarClient, authedUserID *uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := &RemittanceHandler{
		db:            db,
		config:        cfg,
		stellarClient: stellar,
		fees:          services.NewFeeService(cfg),
	}
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	if authedUserID != nil {
		uid := *authedUserID
		r.Use(func(c *gin.Context) {
			c.Set("userID", uid)
			c.Next()
		})
	}
	r.POST("/remittances/batch", handler.CreateBatchRemittance)
	return r
}

func seedBatchUser(t *testing.T, db *gorm.DB, id uint, addr string) models.User {
	t.Helper()
	u := models.User{
		ID:             id,
		Email:          "batch@example.com",
		Name:           "Batch User",
		StellarAddress: addr,
		PasswordHash:   "hashed",
		IsActive:       true,
	}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func TestCreateBatchRemittance(t *testing.T) {
	t.Run("All valid entries create payments in order", func(t *testing.T) {
		db := setupTestDB()
		user := seedBatchUser(t, db, 1, testSenderAddr)
		uid := user.ID
		cfg := &config.Config{MaxBatchSize: 10, PlatformFeeBps: 50}
		stellar := &MockStellarClient{
			ValidateAccountFunc: func(accountID string) error { return nil },
		}
		router := setupBatchRouter(db, cfg, stellar, &uid)

		body, _ := json.Marshal(CreateBatchRemittanceRequest{
			SenderAccount: testSenderAddr,
			AssetCode:     "USDC",
			Payments: []BatchPaymentItem{
				{RecipientAccount: testRecipientAddr1, Amount: 10000, Notes: "first"},
				{RecipientAccount: testRecipientAddr2, Amount: 20000, Notes: "second"},
				{RecipientAccount: testRecipientAddr1, Amount: 5000, Notes: "duplicate recipient allowed"},
			},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/batch", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var resp struct {
			Results []BatchPaymentResult `json:"results"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Results, 3)
		assert.Equal(t, 0, resp.Results[0].Index)
		assert.Equal(t, int64(10000), resp.Results[0].Amount)
		assert.Equal(t, 1, resp.Results[1].Index)
		assert.Equal(t, int64(20000), resp.Results[1].Amount)
		assert.Equal(t, 2, resp.Results[2].Index)
		assert.Equal(t, testRecipientAddr1, resp.Results[2].RecipientAccount)

		var count int64
		db.Model(&models.Payment{}).Count(&count)
		assert.Equal(t, int64(3), count)
	})

	t.Run("One invalid entry creates nothing", func(t *testing.T) {
		db := setupTestDB()
		user := seedBatchUser(t, db, 1, testSenderAddr)
		uid := user.ID
		cfg := &config.Config{MaxBatchSize: 10}
		stellar := &MockStellarClient{
			ValidateAccountFunc: func(accountID string) error {
				if accountID == "BAD_ADDR" {
					return errors.New("invalid account")
				}
				return nil
			},
		}
		router := setupBatchRouter(db, cfg, stellar, &uid)

		body, _ := json.Marshal(CreateBatchRemittanceRequest{
			SenderAccount: testSenderAddr,
			AssetCode:     "USDC",
			Payments: []BatchPaymentItem{
				{RecipientAccount: testRecipientAddr1, Amount: 10000},
				{RecipientAccount: "BAD_ADDR", Amount: 20000},
			},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/batch", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var count int64
		db.Model(&models.Payment{}).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("Failure injected mid-batch rolls back everything", func(t *testing.T) {
		db := setupTestDB()
		user := seedBatchUser(t, db, 1, testSenderAddr)
		uid := user.ID
		cfg := &config.Config{MaxBatchSize: 10}
		stellar := &MockStellarClient{
			ValidateAccountFunc: func(accountID string) error { return nil },
		}

		// Inject a DB failure on the 2nd insert in this transaction.
		insertCalls := 0
		require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_second_payment", func(tx *gorm.DB) {
			if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "payments" {
				insertCalls++
				if insertCalls == 2 {
					tx.AddError(errors.New("injected db failure"))
				}
			}
		}))

		router := setupBatchRouter(db, cfg, stellar, &uid)
		body, _ := json.Marshal(CreateBatchRemittanceRequest{
			SenderAccount: testSenderAddr,
			AssetCode:     "USDC",
			Payments: []BatchPaymentItem{
				{RecipientAccount: testRecipientAddr1, Amount: 10000},
				{RecipientAccount: testRecipientAddr2, Amount: 20000},
			},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/batch", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		var count int64
		db.Model(&models.Payment{}).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("Max batch size passes and max+1 is rejected", func(t *testing.T) {
		db := setupTestDB()
		user := seedBatchUser(t, db, 1, testSenderAddr)
		uid := user.ID
		cfg := &config.Config{MaxBatchSize: 2}
		stellar := &MockStellarClient{
			ValidateAccountFunc: func(accountID string) error { return nil },
		}
		router := setupBatchRouter(db, cfg, stellar, &uid)

		// Exactly 2 passes
		body2, _ := json.Marshal(CreateBatchRemittanceRequest{
			SenderAccount: testSenderAddr,
			AssetCode:     "USDC",
			Payments: []BatchPaymentItem{
				{RecipientAccount: testRecipientAddr1, Amount: 1000},
				{RecipientAccount: testRecipientAddr2, Amount: 2000},
			},
		})
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest("POST", "/remittances/batch", bytes.NewBuffer(body2))
		router.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusCreated, w2.Code)

		// 3 is rejected
		body3, _ := json.Marshal(CreateBatchRemittanceRequest{
			SenderAccount: testSenderAddr,
			AssetCode:     "USDC",
			Payments: []BatchPaymentItem{
				{RecipientAccount: testRecipientAddr1, Amount: 1000},
				{RecipientAccount: testRecipientAddr2, Amount: 2000},
				{RecipientAccount: testRecipientAddr1, Amount: 3000},
			},
		})
		w3 := httptest.NewRecorder()
		req3, _ := http.NewRequest("POST", "/remittances/batch", bytes.NewBuffer(body3))
		router.ServeHTTP(w3, req3)
		assert.Equal(t, http.StatusBadRequest, w3.Code)
	})

	t.Run("Empty batch is rejected", func(t *testing.T) {
		db := setupTestDB()
		user := seedBatchUser(t, db, 1, testSenderAddr)
		uid := user.ID
		cfg := &config.Config{MaxBatchSize: 10}
		stellar := &MockStellarClient{}
		router := setupBatchRouter(db, cfg, stellar, &uid)

		body, _ := json.Marshal(CreateBatchRemittanceRequest{
			SenderAccount: testSenderAddr,
			AssetCode:     "USDC",
			Payments:      []BatchPaymentItem{},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/batch", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Another user sender account is rejected", func(t *testing.T) {
		db := setupTestDB()
		user := seedBatchUser(t, db, 1, testSenderAddr)
		uid := user.ID
		cfg := &config.Config{MaxBatchSize: 10}
		stellar := &MockStellarClient{}
		router := setupBatchRouter(db, cfg, stellar, &uid)

		body, _ := json.Marshal(CreateBatchRemittanceRequest{
			SenderAccount: testOtherAddr,
			AssetCode:     "USDC",
			Payments: []BatchPaymentItem{
				{RecipientAccount: testRecipientAddr1, Amount: 1000},
			},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/batch", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Unauthenticated request is rejected", func(t *testing.T) {
		db := setupTestDB()
		cfg := &config.Config{MaxBatchSize: 10}
		stellar := &MockStellarClient{}
		router := setupBatchRouter(db, cfg, stellar, nil)

		body, _ := json.Marshal(CreateBatchRemittanceRequest{
			SenderAccount: testSenderAddr,
			AssetCode:     "USDC",
			Payments: []BatchPaymentItem{
				{RecipientAccount: testRecipientAddr1, Amount: 1000},
			},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/remittances/batch", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
