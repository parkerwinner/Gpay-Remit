package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/models"
	"github.com/yourusername/gpay-remit/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAnalyticsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = db.AutoMigrate(&models.Payment{})
	assert.NoError(t, err)

	now := time.Now()
	testPayments := []models.Payment{
		{
			SenderID:         1,
			RecipientID:      2,
			Amount:           1000.00,
			Currency:         "USD",
			TargetCurrency:   "EUR",
			ConvertedAmount:  950.00,
			Status:           "completed",
			Fee:              10.00,
			PlatformFee:      5.00,
			ForexFee:         2.50,
			ComplianceFee:    1.00,
			NetworkFee:       1.50,
			CreatedAt:        now.Add(-1 * time.Hour),
		},
		{
			SenderID:         1,
			RecipientID:      3,
			Amount:           2000.00,
			Currency:         "USD",
			TargetCurrency:   "GBP",
			ConvertedAmount:  1600.00,
			Status:           "completed",
			Fee:              20.00,
			PlatformFee:      10.00,
			ForexFee:         5.00,
			ComplianceFee:    2.00,
			NetworkFee:       3.00,
			CreatedAt:        now.Add(-2 * time.Hour),
		},
		{
			SenderID:    2,
			RecipientID: 4,
			Amount:      500.00,
			Currency:    "USD",
			Status:      "failed",
			CreatedAt:   now.Add(-3 * time.Hour),
		},
		{
			SenderID:    3,
			RecipientID: 5,
			Amount:      750.00,
			Currency:    "USD",
			Status:      "pending",
			CreatedAt:   now.Add(-30 * time.Minute),
		},
	}

	for _, payment := range testPayments {
		err = db.Create(&payment).Error
		assert.NoError(t, err)
	}

	return db
}

func TestGetVolumeMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAnalyticsTestDB(t)

	handler := NewAnalyticsHandler(db)

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "Daily volume metrics",
			queryParams:    "?period=daily",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response services.VolumeMetrics
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "daily", response.Period)
				assert.Equal(t, int64(2), response.TotalCount)
				assert.Equal(t, 3000.00, response.TotalVolume)
			},
		},
		{
			name:           "Invalid period",
			queryParams:    "?period=invalid",
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/analytics/volume", handler.GetVolumeMetrics)

			req := httptest.NewRequest(http.MethodGet, "/analytics/volume"+tt.queryParams, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}
}

func TestGetFeeMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAnalyticsTestDB(t)

	handler := NewAnalyticsHandler(db)

	router := gin.New()
	router.GET("/analytics/fees", handler.GetFeeMetrics)

	req := httptest.NewRequest(http.MethodGet, "/analytics/fees?period=daily", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response services.FeeMetrics
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "daily", response.Period)
	assert.Equal(t, int64(2), response.TransactionCount)
	assert.Equal(t, 30.00, response.TotalFees)
	assert.Equal(t, 15.00, response.PlatformFees)
	assert.Equal(t, 7.50, response.ForexFees)
	assert.Equal(t, 3.00, response.ComplianceFees)
	assert.Equal(t, 4.50, response.NetworkFees)
}

func TestGetSuccessRate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAnalyticsTestDB(t)

	handler := NewAnalyticsHandler(db)

	router := gin.New()
	router.GET("/analytics/success-rate", handler.GetSuccessRate)

	req := httptest.NewRequest(http.MethodGet, "/analytics/success-rate?period=daily", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response services.SuccessRateMetrics
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "daily", response.Period)
	assert.Equal(t, int64(4), response.TotalTransactions)
	assert.Equal(t, int64(2), response.SuccessfulTransactions)
	assert.Equal(t, int64(1), response.FailedTransactions)
	assert.Equal(t, int64(1), response.PendingTransactions)
	assert.Equal(t, 50.0, response.SuccessRate)
	assert.Equal(t, 25.0, response.FailureRate)
}

func TestGetTopCorridors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAnalyticsTestDB(t)

	handler := NewAnalyticsHandler(db)

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "Get top corridors with default limit",
			queryParams:    "?period=daily",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, float64(10), response["limit"])
				
				corridors, ok := response["corridors"].([]interface{})
				assert.True(t, ok)
				assert.Equal(t, 2, len(corridors))
			},
		},
		{
			name:           "Get top corridors with custom limit",
			queryParams:    "?period=daily&limit=5",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, float64(5), response["limit"])
			},
		},
		{
			name:           "Invalid limit",
			queryParams:    "?period=daily&limit=200",
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/analytics/top-corridors", handler.GetTopCorridors)

			req := httptest.NewRequest(http.MethodGet, "/analytics/top-corridors"+tt.queryParams, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}
}

func TestIsValidPeriod(t *testing.T) {
	tests := []struct {
		period   string
		expected bool
	}{
		{"daily", true},
		{"weekly", true},
		{"monthly", true},
		{"yearly", true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			result := isValidPeriod(tt.period)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCacheDuration(t *testing.T) {
	tests := []struct {
		period   string
		expected time.Duration
	}{
		{"daily", 5 * time.Minute},
		{"weekly", 15 * time.Minute},
		{"monthly", 30 * time.Minute},
		{"yearly", 1 * time.Hour},
		{"unknown", 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			result := getCacheDuration(tt.period)
			assert.Equal(t, tt.expected, result)
		})
	}
}
