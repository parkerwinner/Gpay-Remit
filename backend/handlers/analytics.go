package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/services"
	"github.com/yourusername/gpay-remit/utils"
	"gorm.io/gorm"
)

type AnalyticsHandler struct {
	service *services.AnalyticsService
}

func NewAnalyticsHandler(db *gorm.DB) *AnalyticsHandler {
	return &AnalyticsHandler{
		service: services.NewAnalyticsService(db),
	}
}

func (h *AnalyticsHandler) GetVolumeMetrics(c *gin.Context) {
	period := c.DefaultQuery("period", "daily")
	
	if !isValidPeriod(period) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid period. Valid values are: daily, weekly, monthly, yearly",
		})
		return
	}

	startDate, endDate, customRange := parseDateRange(c, period)
	if customRange {
		cacheKey := fmt.Sprintf("analytics:volume:%s:%s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
		
		var cachedMetrics services.VolumeMetrics
		found, err := utils.GetCached(cacheKey, &cachedMetrics)
		if err == nil && found {
			c.JSON(http.StatusOK, cachedMetrics)
			return
		}
	} else {
		var err error
		startDate, endDate, err = h.service.CalculateDateRange(period)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		cacheKey := fmt.Sprintf("analytics:volume:%s:%s", period, time.Now().Format("2006-01-02"))
		
		var cachedMetrics services.VolumeMetrics
		found, err := utils.GetCached(cacheKey, &cachedMetrics)
		if err == nil && found {
			c.JSON(http.StatusOK, cachedMetrics)
			return
		}
	}

	metrics, err := h.service.GetVolumeMetrics(period, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve volume metrics"})
		return
	}

	cacheKey := fmt.Sprintf("analytics:volume:%s:%s", period, time.Now().Format("2006-01-02"))
	utils.SetCached(cacheKey, metrics, getCacheDuration(period))

	c.JSON(http.StatusOK, metrics)
}

func (h *AnalyticsHandler) GetFeeMetrics(c *gin.Context) {
	period := c.DefaultQuery("period", "daily")
	
	if !isValidPeriod(period) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid period. Valid values are: daily, weekly, monthly, yearly",
		})
		return
	}

	startDate, endDate, customRange := parseDateRange(c, period)
	if customRange {
		cacheKey := fmt.Sprintf("analytics:fees:%s:%s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
		
		var cachedMetrics services.FeeMetrics
		found, err := utils.GetCached(cacheKey, &cachedMetrics)
		if err == nil && found {
			c.JSON(http.StatusOK, cachedMetrics)
			return
		}
	} else {
		var err error
		startDate, endDate, err = h.service.CalculateDateRange(period)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		cacheKey := fmt.Sprintf("analytics:fees:%s:%s", period, time.Now().Format("2006-01-02"))
		
		var cachedMetrics services.FeeMetrics
		found, err := utils.GetCached(cacheKey, &cachedMetrics)
		if err == nil && found {
			c.JSON(http.StatusOK, cachedMetrics)
			return
		}
	}

	metrics, err := h.service.GetFeeMetrics(period, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve fee metrics"})
		return
	}

	cacheKey := fmt.Sprintf("analytics:fees:%s:%s", period, time.Now().Format("2006-01-02"))
	utils.SetCached(cacheKey, metrics, getCacheDuration(period))

	c.JSON(http.StatusOK, metrics)
}

func (h *AnalyticsHandler) GetSuccessRate(c *gin.Context) {
	period := c.DefaultQuery("period", "daily")
	
	if !isValidPeriod(period) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid period. Valid values are: daily, weekly, monthly, yearly",
		})
		return
	}

	startDate, endDate, customRange := parseDateRange(c, period)
	if customRange {
		cacheKey := fmt.Sprintf("analytics:success_rate:%s:%s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
		
		var cachedMetrics services.SuccessRateMetrics
		found, err := utils.GetCached(cacheKey, &cachedMetrics)
		if err == nil && found {
			c.JSON(http.StatusOK, cachedMetrics)
			return
		}
	} else {
		var err error
		startDate, endDate, err = h.service.CalculateDateRange(period)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		cacheKey := fmt.Sprintf("analytics:success_rate:%s:%s", period, time.Now().Format("2006-01-02"))
		
		var cachedMetrics services.SuccessRateMetrics
		found, err := utils.GetCached(cacheKey, &cachedMetrics)
		if err == nil && found {
			c.JSON(http.StatusOK, cachedMetrics)
			return
		}
	}

	metrics, err := h.service.GetSuccessRateMetrics(period, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve success rate metrics"})
		return
	}

	cacheKey := fmt.Sprintf("analytics:success_rate:%s:%s", period, time.Now().Format("2006-01-02"))
	utils.SetCached(cacheKey, metrics, getCacheDuration(period))

	c.JSON(http.StatusOK, metrics)
}

func (h *AnalyticsHandler) GetTopCorridors(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit. Must be between 1 and 100"})
		return
	}

	period := c.DefaultQuery("period", "monthly")
	if !isValidPeriod(period) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid period. Valid values are: daily, weekly, monthly, yearly",
		})
		return
	}

	startDate, endDate, customRange := parseDateRange(c, period)
	if !customRange {
		startDate, endDate, err = h.service.CalculateDateRange(period)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	cacheKey := fmt.Sprintf("analytics:corridors:%d:%s:%s", limit, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	
	var cachedCorridors []services.CorridorMetrics
	found, err := utils.GetCached(cacheKey, &cachedCorridors)
	if err == nil && found {
		c.JSON(http.StatusOK, gin.H{
			"corridors": cachedCorridors,
			"limit":     limit,
			"period":    period,
		})
		return
	}

	corridors, err := h.service.GetTopCorridors(limit, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve top corridors"})
		return
	}

	utils.SetCached(cacheKey, corridors, getCacheDuration(period))

	c.JSON(http.StatusOK, gin.H{
		"corridors": corridors,
		"limit":     limit,
		"period":    period,
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
	})
}

func isValidPeriod(period string) bool {
	validPeriods := map[string]bool{
		"daily":   true,
		"weekly":  true,
		"monthly": true,
		"yearly":  true,
	}
	return validPeriods[period]
}

func parseDateRange(c *gin.Context, defaultPeriod string) (time.Time, time.Time, bool) {
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	if startDateStr != "" && endDateStr != "" {
		startDate, err1 := time.Parse("2006-01-02", startDateStr)
		endDate, err2 := time.Parse("2006-01-02", endDateStr)
		
		if err1 == nil && err2 == nil {
			return startDate, endDate, true
		}
	}

	return time.Time{}, time.Time{}, false
}

func getCacheDuration(period string) time.Duration {
	switch period {
	case "daily":
		return 5 * time.Minute
	case "weekly":
		return 15 * time.Minute
	case "monthly":
		return 30 * time.Minute
	case "yearly":
		return 1 * time.Hour
	default:
		return 10 * time.Minute
	}
}
