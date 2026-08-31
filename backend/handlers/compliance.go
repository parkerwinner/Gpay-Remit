package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

type ComplianceHandler struct {
	db *gorm.DB
}

func NewComplianceHandler(db *gorm.DB) *ComplianceHandler {
	return &ComplianceHandler{db: db}
}

// GenerateComplianceReport generates AML/KYC reports for auditors
func (h *ComplianceHandler) GenerateComplianceReport(c *gin.Context) {
	// Extract filters
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	minAmountStr := c.Query("min_amount")

	query := h.db.Model(&models.Payment{})

	// Apply date filters
	if startDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", startDate); err == nil {
			query = query.Where("created_at >= ?", parsedDate)
		}
	}
	if endDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", endDate); err == nil {
			query = query.Where("created_at < ?", parsedDate.AddDate(0, 0, 1))
		}
	}

	// Apply amount threshold filter for AML
	if minAmountStr != "" {
		if minAmount, err := strconv.ParseFloat(minAmountStr, 64); err == nil {
			query = query.Where("amount >= ?", minAmount)
		}
	}

	var payments []models.Payment
	if err := query.Order("created_at DESC").Find(&payments).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch compliance data", err))
		return
	}

	if len(payments) == 0 {
		c.Error(errors.NewNotFoundError("No transactions found matching compliance criteria"))
		return
	}

	// Generate CSV
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	header := []string{
		"Transaction ID",
		"Timestamp",
		"Sender ID",
		"Recipient ID",
		"Amount",
		"Currency",
		"Status",
		"Compliance Flag",
	}
	if err := writer.Write(header); err != nil {
		c.Error(errors.NewInternalError("Failed to write CSV header", err))
		return
	}

	for _, p := range payments {
		// Example simplistic flag
		flag := "NORMAL"
		if p.Amount >= 10000 {
			flag = "HIGH_VALUE_REVIEW"
		}
		
		row := []string{
			fmt.Sprintf("%d", p.ID),
			p.CreatedAt.Format(time.RFC3339),
			fmt.Sprintf("%d", p.SenderID),
			fmt.Sprintf("%d", p.RecipientID),
			fmt.Sprintf("%.2f", p.Amount),
			p.Currency,
			p.Status,
			flag,
		}
		if err := writer.Write(row); err != nil {
			c.Error(errors.NewInternalError("Failed to write CSV row", err))
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		c.Error(errors.NewInternalError("Failed to flush CSV writer", err))
		return
	}

	filename := fmt.Sprintf("compliance_report_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Description", "Compliance Report")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "text/csv", buf.Bytes())
}
