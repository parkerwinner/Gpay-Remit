package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type stubRateLookup struct {
	rates map[string]*services.ExchangeRate
}

func (s *stubRateLookup) GetRate(from, to string) (*services.ExchangeRate, error) {
	if r, ok := s.rates[from+"->"+to]; ok {
		return r, nil
	}
	return nil, errors.New("rate unavailable")
}

func setupWalletRouter(db *gorm.DB, stellar *MockStellarClient, rates RateLookup, authedUserID *uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &WalletHandler{
		db:            db,
		config:        &config.Config{},
		stellarClient: stellar,
		rateService:   rates,
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
	r.GET("/wallet/balances", h.GetBalances)
	r.GET("/wallet/preferences", h.GetPreferences)
	r.PUT("/wallet/preferences", h.UpdatePreferences)
	return r
}

func TestWalletBalancesAndPreferences(t *testing.T) {
	t.Run("Multi-currency balances with conversion and missing rate", func(t *testing.T) {
		db := setupTestDB()
		u1, u2 := seedTemplateUsers(t, db)
		require.NoError(t, db.Model(&u1).Update("default_currency", "USD").Error)

		stellar := &MockStellarClient{
			GetAccountBalancesFunc: func(accountID string) ([]utils.AccountBalance, error) {
				if accountID == u2.StellarAddress {
					return []utils.AccountBalance{{AssetCode: "USD", Balance: "9999.00"}}, nil
				}
				return []utils.AccountBalance{
					{AssetCode: "USD", Balance: "100.00"},
					{AssetCode: "EUR", Balance: "50.00"},
					{AssetCode: "XLM", Balance: "200.0000000"},
				}, nil
			},
		}
		ts := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
		rates := &stubRateLookup{
			rates: map[string]*services.ExchangeRate{
				"EUR->USD": {From: "EUR", To: "USD", Rate: 1100000, Scale: 1000000, Timestamp: ts, Origin: "cache"},
			},
		}

		uid := u1.ID
		router := setupWalletRouter(db, stellar, rates, &uid)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/wallet/balances", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Balances               []WalletBalanceItem `json:"balances"`
			PreferredCurrency      string              `json:"preferred_currency"`
			SupportedCurrencies    []string            `json:"supported_currencies"`
			EstimatedTotal         string              `json:"estimated_total"`
			HasUnconvertedBalances bool                `json:"has_unconverted_balances"`
			IsEstimate             bool                `json:"is_estimate"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "USD", resp.PreferredCurrency)
		assert.NotEmpty(t, resp.SupportedCurrencies)
		assert.True(t, resp.IsEstimate)
		assert.True(t, resp.HasUnconvertedBalances)
		// 100.00 USD + (50.00 * 1.10) = 155.00; XLM excluded due to missing rate
		assert.Equal(t, "155.00", resp.EstimatedTotal)
		assert.NotContains(t, w.Body.String(), "9999.00")
	})

	t.Run("No balances returns empty list", func(t *testing.T) {
		db := setupTestDB()
		u1, _ := seedTemplateUsers(t, db)
		uid := u1.ID
		stellar := &MockStellarClient{
			GetAccountBalancesFunc: func(accountID string) ([]utils.AccountBalance, error) {
				return []utils.AccountBalance{}, nil
			},
		}
		router := setupWalletRouter(db, stellar, nil, &uid)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/wallet/balances", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"balances":[]`)
	})

	t.Run("Get and update currency preference with invalid rejected", func(t *testing.T) {
		db := setupTestDB()
		u1, _ := seedTemplateUsers(t, db)
		uid := u1.ID
		router := setupWalletRouter(db, &MockStellarClient{}, nil, &uid)

		// Invalid currency rejected
		badBody, _ := json.Marshal(UpdateWalletPreferenceRequest{PreferredCurrency: "INVALID"})
		wBad := httptest.NewRecorder()
		reqBad, _ := http.NewRequest("PUT", "/wallet/preferences", bytes.NewBuffer(badBody))
		router.ServeHTTP(wBad, reqBad)
		assert.Equal(t, http.StatusBadRequest, wBad.Code)

		// Valid currency persisted
		goodBody, _ := json.Marshal(UpdateWalletPreferenceRequest{PreferredCurrency: "EUR"})
		wGood := httptest.NewRecorder()
		reqGood, _ := http.NewRequest("PUT", "/wallet/preferences", bytes.NewBuffer(goodBody))
		router.ServeHTTP(wGood, reqGood)
		assert.Equal(t, http.StatusOK, wGood.Code)

		wGet := httptest.NewRecorder()
		reqGet, _ := http.NewRequest("GET", "/wallet/preferences", nil)
		router.ServeHTTP(wGet, reqGet)
		assert.Equal(t, http.StatusOK, wGet.Code)
		assert.Contains(t, wGet.Body.String(), `"preferred_currency":"EUR"`)
	})

	t.Run("Unauthenticated returns 401", func(t *testing.T) {
		db := setupTestDB()
		router := setupWalletRouter(db, &MockStellarClient{}, nil, nil)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/wallet/balances", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
