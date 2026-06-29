package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupExportTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&models.Payment{})
	return db
}

func seedTestPayments(db *gorm.DB, count int) []models.Payment {
	payments := make([]models.Payment, count)
	for i := 0; i < count; i++ {
		payment := models.Payment{
			SenderID:        uint(i + 1),
			SenderAccount:   fmt.Sprintf("SENDER%d", i+1),
			RecipientID:     uint(i + 100),
			RecipientAccount: fmt.Sprintf("RECIPIENT%d", i+1),
			Amount:          float64(100 + i*10),
			Currency:        "USD",
			TargetCurrency:  "EUR",
			ConvertedAmount: float64(90 + i*9),
			Status:          "completed",
			Fee:             2.5,
			PlatformFee:     1.0,
			ForexFee:        0.5,
			ComplianceFee:   0.5,
			NetworkFee:      0.5,
			TxHash:          fmt.Sprintf("hash%d", i),
			EscrowID:        fmt.Sprintf("escrow%d", i),
			Notes:           fmt.Sprintf("Test payment %d", i),
			CreatedAt:       time.Now().Add(time.Duration(-i) * time.Hour),
		}
		db.Create(&payment)
		payments[i] = payment
	}
	return payments
}

func TestExportTransactionsCSV(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupExportTestDB()
	seedTestPayments(db, 5)

	handler := NewExportHandler(db)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/api/v1/transactions/export", handler.ExportTransactions)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/transactions/export?format=csv", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".csv")

	// Parse CSV and verify content
	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, err := reader.ReadAll()
	assert.NoError(t, err)
	assert.Greater(t, len(records), 1) // At least header + 1 row

	// Check header
	header := records[0]
	assert.Equal(t, "ID", header[0])
	assert.Equal(t, "Created At", header[1])
	assert.Equal(t, "Amount", header[6])
	assert.Equal(t, "Total Fee", header[11])

	// Check data row
	assert.Equal(t, 6, len(records)) // 1 header + 5 data rows
}

func TestExportTransactionsPDF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupExportTestDB()
	seedTestPayments(db, 3)

	handler := NewExportHandler(db)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/api/v1/transactions/export", handler.ExportTransactions)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/transactions/export?format=pdf", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".pdf")

	// Verify PDF magic number
	body := w.Body.Bytes()
	assert.True(t, len(body) > 4)
	assert.Equal(t, "%PDF", string(body[:4]))
}

func TestExportTransactionsWithFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupExportTestDB()
	
	// Create payments with different statuses and dates
	db.Create(&models.Payment{
		SenderID: 1, RecipientID: 2, Amount: 100, Currency: "USD",
		Status: "completed", CreatedAt: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
	})
	db.Create(&models.Payment{
		SenderID: 1, RecipientID: 2, Amount: 200, Currency: "EUR",
		Status: "pending", CreatedAt: time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC),
	})
	db.Create(&models.Payment{
		SenderID: 1, RecipientID: 2, Amount: 300, Currency: "USD",
		Status: "completed", CreatedAt: time.Date(2024, 2, 10, 0, 0, 0, 0, time.UTC),
	})

	handler := NewExportHandler(db)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/api/v1/transactions/export", handler.ExportTransactions)

	// Test status filter
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/transactions/export?format=csv&status=completed", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, _ := reader.ReadAll()
	assert.Equal(t, 3, len(records)) // 1 header + 2 completed payments

	// Test date range filter
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/transactions/export?format=csv&start_date=2024-01-01&end_date=2024-01-31", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	reader = csv.NewReader(strings.NewReader(w.Body.String()))
	records, _ = reader.ReadAll()
	assert.Equal(t, 3, len(records)) // 1 header + 2 January payments

	// Test currency filter
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/transactions/export?format=csv&currency=EUR", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	reader = csv.NewReader(strings.NewReader(w.Body.String()))
	records, _ = reader.ReadAll()
	assert.Equal(t, 2, len(records)) // 1 header + 1 EUR payment
}

func TestExportTransactionsInvalidFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupExportTestDB()
	seedTestPayments(db, 1)

	handler := NewExportHandler(db)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(errorHandlerForTests())
	router.GET("/api/v1/transactions/export", handler.ExportTransactions)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/transactions/export?format=xml", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExportTransactionsNoData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupExportTestDB()

	handler := NewExportHandler(db)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(errorHandlerForTests())
	router.GET("/api/v1/transactions/export", handler.ExportTransactions)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/transactions/export?format=csv", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestExportTransactionsPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupExportTestDB()
	seedTestPayments(db, 25)

	handler := NewExportHandler(db)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/api/v1/transactions/export", handler.ExportTransactions)

	// Test first page
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/transactions/export?format=csv&page=1&page_size=10", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	records, _ := reader.ReadAll()
	assert.Equal(t, 11, len(records)) // 1 header + 10 data rows

	// Test second page
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/transactions/export?format=csv&page=2&page_size=10", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	reader = csv.NewReader(strings.NewReader(w.Body.String()))
	records, _ = reader.ReadAll()
	assert.Equal(t, 11, len(records)) // 1 header + 10 data rows
}
