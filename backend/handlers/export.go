package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jung-kurt/gofpdf"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

type ExportHandler struct {
	db *gorm.DB
}

func NewExportHandler(db *gorm.DB) *ExportHandler {
	return &ExportHandler{db: db}
}

// ExportTransactions handles both CSV and PDF export requests
func (h *ExportHandler) ExportTransactions(c *gin.Context) {
	format := c.Query("format")
	if format != "csv" && format != "pdf" {
		c.Error(errors.NewValidationError("Invalid format", "format must be 'csv' or 'pdf'"))
		return
	}

	// Extract filters from query params
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	status := c.Query("status")
	currency := c.Query("currency")
	page := c.DefaultQuery("page", "1")
	pageSize := c.DefaultQuery("page_size", "1000")

	// Build query
	query := h.db.Model(&models.Payment{})

	// Apply date filters
	if startDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", startDate); err == nil {
			query = query.Where("created_at >= ?", parsedDate)
		}
	}
	if endDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", endDate); err == nil {
			// Add 1 day to include the entire end date
			query = query.Where("created_at < ?", parsedDate.AddDate(0, 0, 1))
		}
	}

	// Apply status filter
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Apply currency filter
	if currency != "" {
		query = query.Where("currency = ?", currency)
	}

	// Apply pagination
	pageNum, _ := strconv.Atoi(page)
	pageSizeNum, _ := strconv.Atoi(pageSize)
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSizeNum <= 0 || pageSizeNum > 10000 {
		pageSizeNum = 1000
	}
	offset := (pageNum - 1) * pageSizeNum

	// Fetch payments
	var payments []models.Payment
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSizeNum).Find(&payments).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to fetch transactions", err))
		return
	}

	if len(payments) == 0 {
		c.Error(errors.NewNotFoundError("No transactions found"))
		return
	}

	// Generate export based on format
	if format == "csv" {
		h.exportCSV(c, payments)
	} else {
		h.exportPDF(c, payments)
	}
}

func (h *ExportHandler) exportCSV(c *gin.Context, payments []models.Payment) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	header := []string{
		"ID",
		"Created At",
		"Sender ID",
		"Sender Account",
		"Recipient ID",
		"Recipient Account",
		"Amount",
		"Currency",
		"Target Currency",
		"Converted Amount",
		"Status",
		"Total Fee",
		"Platform Fee",
		"Forex Fee",
		"Compliance Fee",
		"Network Fee",
		"TX Hash",
		"Escrow ID",
		"Notes",
	}
	if err := writer.Write(header); err != nil {
		c.Error(errors.NewInternalError("Failed to write CSV header", err))
		return
	}

	// Write data rows
	for _, payment := range payments {
		row := []string{
			fmt.Sprintf("%d", payment.ID),
			payment.CreatedAt.Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%d", payment.SenderID),
			payment.SenderAccount,
			fmt.Sprintf("%d", payment.RecipientID),
			payment.RecipientAccount,
			fmt.Sprintf("%.2f", payment.Amount),
			payment.Currency,
			payment.TargetCurrency,
			fmt.Sprintf("%.2f", payment.ConvertedAmount),
			payment.Status,
			fmt.Sprintf("%.4f", payment.Fee),
			fmt.Sprintf("%.4f", payment.PlatformFee),
			fmt.Sprintf("%.4f", payment.ForexFee),
			fmt.Sprintf("%.4f", payment.ComplianceFee),
			fmt.Sprintf("%.4f", payment.NetworkFee),
			payment.TxHash,
			payment.EscrowID,
			payment.Notes,
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

	// Set response headers
	filename := fmt.Sprintf("transactions_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "text/csv", buf.Bytes())
}

func (h *ExportHandler) exportPDF(c *gin.Context, payments []models.Payment) {
	pdf := gofpdf.New("L", "mm", "A4", "")
	pdf.AddPage()

	// Title
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(0, 10, "Transaction Export Report")
	pdf.Ln(12)

	// Metadata
	pdf.SetFont("Arial", "", 10)
	pdf.Cell(0, 6, fmt.Sprintf("Generated: %s", time.Now().Format("2006-01-02 15:04:05")))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Total Transactions: %d", len(payments)))
	pdf.Ln(10)

	// Table header
	pdf.SetFont("Arial", "B", 8)
	pdf.SetFillColor(200, 220, 255)
	
	// Column widths (total: 277mm for A4 landscape)
	widths := []float64{10, 35, 20, 20, 20, 15, 15, 15, 15, 15, 15, 20, 52}
	headers := []string{
		"ID",
		"Date",
		"Sender",
		"Recipient",
		"Amount",
		"Status",
		"Fee",
		"Platform",
		"Forex",
		"Comp.",
		"Network",
		"TX Hash",
		"Notes",
	}

	for i, header := range headers {
		pdf.CellFormat(widths[i], 7, header, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// Table rows
	pdf.SetFont("Arial", "", 7)
	pdf.SetFillColor(240, 240, 240)
	fill := false

	for _, payment := range payments {
		// Truncate long values
		txHash := payment.TxHash
		if len(txHash) > 10 {
			txHash = txHash[:10] + "..."
		}
		notes := payment.Notes
		if len(notes) > 30 {
			notes = notes[:30] + "..."
		}

		data := []string{
			fmt.Sprintf("%d", payment.ID),
			payment.CreatedAt.Format("2006-01-02"),
			fmt.Sprintf("%d", payment.SenderID),
			fmt.Sprintf("%d", payment.RecipientID),
			fmt.Sprintf("%.2f %s", payment.Amount, payment.Currency),
			payment.Status,
			fmt.Sprintf("%.4f", payment.Fee),
			fmt.Sprintf("%.4f", payment.PlatformFee),
			fmt.Sprintf("%.4f", payment.ForexFee),
			fmt.Sprintf("%.4f", payment.ComplianceFee),
			fmt.Sprintf("%.4f", payment.NetworkFee),
			txHash,
			notes,
		}

		for i, cell := range data {
			pdf.CellFormat(widths[i], 6, cell, "1", 0, "L", fill, 0, "")
		}
		pdf.Ln(-1)
		fill = !fill

		// Add new page if needed
		if pdf.GetY() > 180 {
			pdf.AddPage()
			// Redraw header
			pdf.SetFont("Arial", "B", 8)
			for i, header := range headers {
				pdf.CellFormat(widths[i], 7, header, "1", 0, "C", true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetFont("Arial", "", 7)
		}
	}

	// Summary section
	pdf.Ln(10)
	pdf.SetFont("Arial", "B", 10)
	pdf.Cell(0, 6, "Summary")
	pdf.Ln(8)

	pdf.SetFont("Arial", "", 9)
	
	var totalAmount, totalFees float64
	statusCounts := make(map[string]int)
	
	for _, p := range payments {
		totalAmount += p.Amount
		totalFees += p.Fee
		statusCounts[p.Status]++
	}

	pdf.Cell(0, 6, fmt.Sprintf("Total Transaction Amount: %.2f", totalAmount))
	pdf.Ln(6)
	pdf.Cell(0, 6, fmt.Sprintf("Total Fees Collected: %.4f", totalFees))
	pdf.Ln(6)
	
	pdf.Cell(0, 6, "Status Breakdown:")
	pdf.Ln(6)
	for status, count := range statusCounts {
		pdf.Cell(0, 5, fmt.Sprintf("  - %s: %d transactions", status, count))
		pdf.Ln(5)
	}

	// Generate PDF bytes
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		c.Error(errors.NewInternalError("Failed to generate PDF", err))
		return
	}

	// Set response headers
	filename := fmt.Sprintf("transactions_%s.pdf", time.Now().Format("20060102_150405"))
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "application/pdf", buf.Bytes())
}
