package services

import (
	"fmt"
	"time"

	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

type AnalyticsService struct {
	db *gorm.DB
}

type VolumeMetrics struct {
	Period       string  `json:"period"`
	TotalVolume  float64 `json:"total_volume"`
	TotalCount   int64   `json:"total_count"`
	Currency     string  `json:"currency"`
	StartDate    string  `json:"start_date"`
	EndDate      string  `json:"end_date"`
}

type FeeMetrics struct {
	Period          string  `json:"period"`
	TotalFees       float64 `json:"total_fees"`
	PlatformFees    float64 `json:"platform_fees"`
	ForexFees       float64 `json:"forex_fees"`
	ComplianceFees  float64 `json:"compliance_fees"`
	NetworkFees     float64 `json:"network_fees"`
	TransactionCount int64  `json:"transaction_count"`
	StartDate       string  `json:"start_date"`
	EndDate         string  `json:"end_date"`
}

type SuccessRateMetrics struct {
	Period          string  `json:"period"`
	TotalTransactions int64  `json:"total_transactions"`
	SuccessfulTransactions int64 `json:"successful_transactions"`
	FailedTransactions int64 `json:"failed_transactions"`
	PendingTransactions int64 `json:"pending_transactions"`
	SuccessRate     float64 `json:"success_rate"`
	FailureRate     float64 `json:"failure_rate"`
	StartDate       string  `json:"start_date"`
	EndDate         string  `json:"end_date"`
}

type CorridorMetrics struct {
	SourceCurrency      string  `json:"source_currency"`
	DestinationCurrency string  `json:"destination_currency"`
	TransactionCount    int64   `json:"transaction_count"`
	TotalVolume         float64 `json:"total_volume"`
	AverageAmount       float64 `json:"average_amount"`
	TotalFees           float64 `json:"total_fees"`
}

func NewAnalyticsService(db *gorm.DB) *AnalyticsService {
	return &AnalyticsService{db: db}
}

func (s *AnalyticsService) GetVolumeMetrics(period string, startDate, endDate time.Time) (*VolumeMetrics, error) {
	var result struct {
		TotalVolume float64
		TotalCount  int64
	}

	err := s.db.Model(&models.Payment{}).
		Select("COALESCE(SUM(amount), 0) as total_volume, COUNT(*) as total_count").
		Where("created_at >= ? AND created_at <= ?", startDate, endDate).
		Where("status = ?", "completed").
		Scan(&result).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get volume metrics: %w", err)
	}

	return &VolumeMetrics{
		Period:      period,
		TotalVolume: result.TotalVolume,
		TotalCount:  result.TotalCount,
		Currency:    "USD",
		StartDate:   startDate.Format("2006-01-02"),
		EndDate:     endDate.Format("2006-01-02"),
	}, nil
}

func (s *AnalyticsService) GetFeeMetrics(period string, startDate, endDate time.Time) (*FeeMetrics, error) {
	var result struct {
		TotalFees      float64
		PlatformFees   float64
		ForexFees      float64
		ComplianceFees float64
		NetworkFees    float64
		TotalCount     int64
	}

	err := s.db.Model(&models.Payment{}).
		Select(`
			COALESCE(SUM(fee), 0) as total_fees,
			COALESCE(SUM(platform_fee), 0) as platform_fees,
			COALESCE(SUM(forex_fee), 0) as forex_fees,
			COALESCE(SUM(compliance_fee), 0) as compliance_fees,
			COALESCE(SUM(network_fee), 0) as network_fees,
			COUNT(*) as total_count
		`).
		Where("created_at >= ? AND created_at <= ?", startDate, endDate).
		Where("status = ?", "completed").
		Scan(&result).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get fee metrics: %w", err)
	}

	return &FeeMetrics{
		Period:           period,
		TotalFees:        result.TotalFees,
		PlatformFees:     result.PlatformFees,
		ForexFees:        result.ForexFees,
		ComplianceFees:   result.ComplianceFees,
		NetworkFees:      result.NetworkFees,
		TransactionCount: result.TotalCount,
		StartDate:        startDate.Format("2006-01-02"),
		EndDate:          endDate.Format("2006-01-02"),
	}, nil
}

func (s *AnalyticsService) GetSuccessRateMetrics(period string, startDate, endDate time.Time) (*SuccessRateMetrics, error) {
	var total int64
	err := s.db.Model(&models.Payment{}).
		Where("created_at >= ? AND created_at <= ?", startDate, endDate).
		Count(&total).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count total transactions: %w", err)
	}

	var successful int64
	err = s.db.Model(&models.Payment{}).
		Where("created_at >= ? AND created_at <= ?", startDate, endDate).
		Where("status = ?", "completed").
		Count(&successful).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count successful transactions: %w", err)
	}

	var failed int64
	err = s.db.Model(&models.Payment{}).
		Where("created_at >= ? AND created_at <= ?", startDate, endDate).
		Where("status = ?", "failed").
		Count(&failed).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count failed transactions: %w", err)
	}

	var pending int64
	err = s.db.Model(&models.Payment{}).
		Where("created_at >= ? AND created_at <= ?", startDate, endDate).
		Where("status IN ?", []string{"pending", "processing"}).
		Count(&pending).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count pending transactions: %w", err)
	}

	successRate := 0.0
	failureRate := 0.0
	if total > 0 {
		successRate = float64(successful) / float64(total) * 100
		failureRate = float64(failed) / float64(total) * 100
	}

	return &SuccessRateMetrics{
		Period:                 period,
		TotalTransactions:      total,
		SuccessfulTransactions: successful,
		FailedTransactions:     failed,
		PendingTransactions:    pending,
		SuccessRate:            successRate,
		FailureRate:            failureRate,
		StartDate:              startDate.Format("2006-01-02"),
		EndDate:                endDate.Format("2006-01-02"),
	}, nil
}

func (s *AnalyticsService) GetTopCorridors(limit int, startDate, endDate time.Time) ([]CorridorMetrics, error) {
	var corridors []CorridorMetrics

	err := s.db.Model(&models.Payment{}).
		Select(`
			currency as source_currency,
			target_currency as destination_currency,
			COUNT(*) as transaction_count,
			COALESCE(SUM(amount), 0) as total_volume,
			COALESCE(AVG(amount), 0) as average_amount,
			COALESCE(SUM(fee), 0) as total_fees
		`).
		Where("created_at >= ? AND created_at <= ?", startDate, endDate).
		Where("status = ?", "completed").
		Where("target_currency != ''").
		Group("currency, target_currency").
		Order("transaction_count DESC").
		Limit(limit).
		Scan(&corridors).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get top corridors: %w", err)
	}

	return corridors, nil
}

func (s *AnalyticsService) CalculateDateRange(period string) (time.Time, time.Time, error) {
	now := time.Now()
	var startDate, endDate time.Time

	switch period {
	case "daily":
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		endDate = startDate.Add(24 * time.Hour)
	case "weekly":
		weekday := now.Weekday()
		daysToMonday := (int(weekday) - int(time.Monday) + 7) % 7
		startDate = now.AddDate(0, 0, -daysToMonday)
		startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
		endDate = startDate.Add(7 * 24 * time.Hour)
	case "monthly":
		startDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endDate = startDate.AddDate(0, 1, 0)
	case "yearly":
		startDate = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		endDate = startDate.AddDate(1, 0, 0)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("invalid period: %s", period)
	}

	return startDate, endDate, nil
}
