package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = db.AutoMigrate(&models.Payment{})
	assert.NoError(t, err)

	return db
}

func seedTestData(t *testing.T, db *gorm.DB) {
	now := time.Now()
	
	testPayments := []models.Payment{
		{
			SenderID:        1,
			RecipientID:     2,
			Amount:          1000.00,
			Currency:        "USD",
			TargetCurrency:  "EUR",
			ConvertedAmount: 950.00,
			Status:          "completed",
			Fee:             10.00,
			PlatformFee:     5.00,
			ForexFee:        2.50,
			ComplianceFee:   1.00,
			NetworkFee:      1.50,
			CreatedAt:       now.Add(-1 * time.Hour),
		},
		{
			SenderID:        1,
			RecipientID:     3,
			Amount:          2000.00,
			Currency:        "USD",
			TargetCurrency:  "GBP",
			ConvertedAmount: 1600.00,
			Status:          "completed",
			Fee:             20.00,
			PlatformFee:     10.00,
			ForexFee:        5.00,
			ComplianceFee:   2.00,
			NetworkFee:      3.00,
			CreatedAt:       now.Add(-2 * time.Hour),
		},
		{
			SenderID:        1,
			RecipientID:     2,
			Amount:          1500.00,
			Currency:        "USD",
			TargetCurrency:  "EUR",
			ConvertedAmount: 1425.00,
			Status:          "completed",
			Fee:             15.00,
			PlatformFee:     7.50,
			ForexFee:        3.75,
			ComplianceFee:   1.50,
			NetworkFee:      2.25,
			CreatedAt:       now.Add(-3 * time.Hour),
		},
		{
			SenderID:    2,
			RecipientID: 4,
			Amount:      500.00,
			Currency:    "USD",
			Status:      "failed",
			CreatedAt:   now.Add(-4 * time.Hour),
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
		err := db.Create(&payment).Error
		assert.NoError(t, err)
	}
}

func TestGetVolumeMetrics(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	service := NewAnalyticsService(db)

	now := time.Now()
	startDate := now.Add(-24 * time.Hour)
	endDate := now.Add(1 * time.Hour)

	metrics, err := service.GetVolumeMetrics("daily", startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, "daily", metrics.Period)
	assert.Equal(t, int64(3), metrics.TotalCount)
	assert.Equal(t, 4500.00, metrics.TotalVolume)
	assert.Equal(t, "USD", metrics.Currency)
}

func TestGetFeeMetrics(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	service := NewAnalyticsService(db)

	now := time.Now()
	startDate := now.Add(-24 * time.Hour)
	endDate := now.Add(1 * time.Hour)

	metrics, err := service.GetFeeMetrics("daily", startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, "daily", metrics.Period)
	assert.Equal(t, int64(3), metrics.TransactionCount)
	assert.Equal(t, 45.00, metrics.TotalFees)
	assert.Equal(t, 22.50, metrics.PlatformFees)
	assert.Equal(t, 11.25, metrics.ForexFees)
	assert.Equal(t, 4.50, metrics.ComplianceFees)
	assert.Equal(t, 6.75, metrics.NetworkFees)
}

func TestGetSuccessRateMetrics(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	service := NewAnalyticsService(db)

	now := time.Now()
	startDate := now.Add(-24 * time.Hour)
	endDate := now.Add(1 * time.Hour)

	metrics, err := service.GetSuccessRateMetrics("daily", startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, "daily", metrics.Period)
	assert.Equal(t, int64(5), metrics.TotalTransactions)
	assert.Equal(t, int64(3), metrics.SuccessfulTransactions)
	assert.Equal(t, int64(1), metrics.FailedTransactions)
	assert.Equal(t, int64(1), metrics.PendingTransactions)
	assert.Equal(t, 60.0, metrics.SuccessRate)
	assert.Equal(t, 20.0, metrics.FailureRate)
}

func TestGetTopCorridors(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	service := NewAnalyticsService(db)

	now := time.Now()
	startDate := now.Add(-24 * time.Hour)
	endDate := now.Add(1 * time.Hour)

	corridors, err := service.GetTopCorridors(10, startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, corridors)
	assert.Equal(t, 2, len(corridors))

	assert.Equal(t, "USD", corridors[0].SourceCurrency)
	assert.Equal(t, "EUR", corridors[0].DestinationCurrency)
	assert.Equal(t, int64(2), corridors[0].TransactionCount)
	assert.Equal(t, 2500.00, corridors[0].TotalVolume)
	assert.Equal(t, 25.00, corridors[0].TotalFees)

	assert.Equal(t, "USD", corridors[1].SourceCurrency)
	assert.Equal(t, "GBP", corridors[1].DestinationCurrency)
	assert.Equal(t, int64(1), corridors[1].TransactionCount)
	assert.Equal(t, 2000.00, corridors[1].TotalVolume)
}

func TestCalculateDateRange(t *testing.T) {
	service := NewAnalyticsService(nil)

	tests := []struct {
		name          string
		period        string
		expectError   bool
		validateRange func(*testing.T, time.Time, time.Time)
	}{
		{
			name:        "Daily period",
			period:      "daily",
			expectError: false,
			validateRange: func(t *testing.T, start, end time.Time) {
				diff := end.Sub(start)
				assert.Equal(t, 24*time.Hour, diff)
			},
		},
		{
			name:        "Weekly period",
			period:      "weekly",
			expectError: false,
			validateRange: func(t *testing.T, start, end time.Time) {
				diff := end.Sub(start)
				assert.Equal(t, 7*24*time.Hour, diff)
			},
		},
		{
			name:        "Monthly period",
			period:      "monthly",
			expectError: false,
			validateRange: func(t *testing.T, start, end time.Time) {
				assert.Equal(t, 1, start.Day())
				assert.NotEqual(t, start.Month(), end.Month())
			},
		},
		{
			name:        "Yearly period",
			period:      "yearly",
			expectError: false,
			validateRange: func(t *testing.T, start, end time.Time) {
				assert.Equal(t, 1, int(start.Month()))
				assert.Equal(t, 1, start.Day())
				assert.NotEqual(t, start.Year(), end.Year())
			},
		},
		{
			name:        "Invalid period",
			period:      "invalid",
			expectError: true,
			validateRange: func(t *testing.T, start, end time.Time) {
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := service.CalculateDateRange(tt.period)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.False(t, start.IsZero())
				assert.False(t, end.IsZero())
				assert.True(t, end.After(start))
				
				if tt.validateRange != nil {
					tt.validateRange(t, start, end)
				}
			}
		})
	}
}

func TestGetVolumeMetrics_EmptyData(t *testing.T) {
	db := setupTestDB(t)
	service := NewAnalyticsService(db)

	now := time.Now()
	startDate := now.Add(-24 * time.Hour)
	endDate := now.Add(1 * time.Hour)

	metrics, err := service.GetVolumeMetrics("daily", startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.TotalCount)
	assert.Equal(t, 0.0, metrics.TotalVolume)
}

func TestGetTopCorridors_WithLimit(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	service := NewAnalyticsService(db)

	now := time.Now()
	startDate := now.Add(-24 * time.Hour)
	endDate := now.Add(1 * time.Hour)

	corridors, err := service.GetTopCorridors(1, startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, corridors)
	assert.Equal(t, 1, len(corridors))
	assert.Equal(t, "USD", corridors[0].SourceCurrency)
	assert.Equal(t, "EUR", corridors[0].DestinationCurrency)
}
