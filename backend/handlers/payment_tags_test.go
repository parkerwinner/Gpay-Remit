package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTagTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Payment{}, &models.PaymentTag{}))
	return db
}

func setupTagRouter(db *gorm.DB, authUserID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	h := &RemittanceHandler{
		db:     db,
		readDB: db,
		config: cfg,
		fees:   services.NewFeeService(cfg),
	}
	r := gin.New()
	r.Use(errorHandlerForTests())
	if authUserID > 0 {
		r.Use(func(c *gin.Context) {
			c.Set("userID", authUserID)
			c.Next()
		})
	}
	r.POST("/remittances", h.SendRemittance)
	r.GET("/remittances", h.ListRemittances)
	r.GET("/remittances/:id", h.GetRemittance)
	r.PUT("/remittances/:id/tags", h.UpdateRemittanceTags)
	r.GET("/remittances/tags/summary", h.GetTagSummary)
	r.GET("/remittances/tags/autocomplete", h.AutocompleteTags)
	return r
}

func TestPaymentTags_CreateReplaceAndEmptyArrayJSON(t *testing.T) {
	db := setupTagTestDB(t)
	r := setupTagRouter(db, 1)

	// Create payment without tags -> JSON has "tags": [] (never null)
	body1 := `{"sender_id":1,"recipient_id":2,"amount":500,"currency":"USD","notes":"plain"}`
	req1 := httptest.NewRequest(http.MethodPost, "/remittances", bytes.NewBufferString(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusCreated, w1.Code)
	assert.Contains(t, w1.Body.String(), `"tags":[]`)

	// Create payment with tags -> normalized & deduplicated
	body2 := `{"sender_id":1,"recipient_id":2,"amount":1000,"currency":"XLM","tags":["  Family ","family","Rent-2026"]}`
	req2 := httptest.NewRequest(http.MethodPost, "/remittances", bytes.NewBufferString(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusCreated, w2.Code)

	var created models.Payment
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &created))
	assert.Equal(t, []string{"family", "rent-2026"}, created.Tags)

	// Replace tags via PUT /remittances/:id/tags
	replaceBody := `{"tags":["Utilities","Home_Bills"]}`
	req3 := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/remittances/%d/tags", created.ID), bytes.NewBufferString(replaceBody))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	require.Equal(t, http.StatusOK, w3.Code)

	// Verify GET /remittances/:id returns replaced tags
	req4 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/remittances/%d", created.ID), nil)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	require.Equal(t, http.StatusOK, w4.Code)
	var fetched models.Payment
	require.NoError(t, json.Unmarshal(w4.Body.Bytes(), &fetched))
	assert.Equal(t, []string{"home_bills", "utilities"}, fetched.Tags)
}

func TestPaymentTags_HistoryFilterANDSemanticsAndPagination(t *testing.T) {
	db := setupTagTestDB(t)
	r := setupTagRouter(db, 1)

	p1 := models.Payment{SenderID: 1, RecipientID: 2, Amount: 100, Currency: "USD", Status: "completed"}
	p2 := models.Payment{SenderID: 1, RecipientID: 2, Amount: 200, Currency: "USD", Status: "completed"}
	p3 := models.Payment{SenderID: 99, RecipientID: 100, Amount: 300, Currency: "USD", Status: "completed"} // other user
	require.NoError(t, db.Create(&p1).Error)
	require.NoError(t, db.Create(&p2).Error)
	require.NoError(t, db.Create(&p3).Error)

	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p1.ID, UserID: 1, Tag: "family"}).Error)
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p1.ID, UserID: 1, Tag: "urgent"}).Error)
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p2.ID, UserID: 1, Tag: "family"}).Error)
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p3.ID, UserID: 99, Tag: "family"}).Error)

	// Single tag -> returns p1 and p2 (never p3)
	req := httptest.NewRequest(http.MethodGet, "/remittances?tag=family&page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp ListRemittancesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.TotalCount)
	assert.Equal(t, int64(2), *resp.TotalCount)
	assert.Len(t, resp.Data, 2)

	// Multiple tags (AND semantics) -> returns only p1
	req2 := httptest.NewRequest(http.MethodGet, "/remittances?tag=family&tag=urgent&page=1&page_size=10", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	var resp2 ListRemittancesResponse
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	require.NotNil(t, resp2.TotalCount)
	assert.Equal(t, int64(1), *resp2.TotalCount)
	assert.Len(t, resp2.Data, 1)
	assert.Equal(t, p1.ID, resp2.Data[0].ID)

	// Unknown tag -> empty page
	req3 := httptest.NewRequest(http.MethodGet, "/remittances?tag=nonexistent", nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	require.Equal(t, http.StatusOK, w3.Code)
	var resp3 ListRemittancesResponse
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &resp3))
	assert.Empty(t, resp3.Data)
}

func TestPaymentTags_SummaryPerAssetAndCompletedOnly(t *testing.T) {
	db := setupTagTestDB(t)
	r := setupTagRouter(db, 1)

	p1 := models.Payment{SenderID: 1, RecipientID: 2, Amount: 1000, Currency: "USD", Status: "completed"}
	p2 := models.Payment{SenderID: 1, RecipientID: 2, Amount: 2500, Currency: "XLM", Status: "completed"}
	p3 := models.Payment{SenderID: 1, RecipientID: 2, Amount: 9999, Currency: "USD", Status: "pending"} // excluded
	p4 := models.Payment{SenderID: 2, RecipientID: 3, Amount: 5000, Currency: "USD", Status: "completed"} // other user
	require.NoError(t, db.Create(&p1).Error)
	require.NoError(t, db.Create(&p2).Error)
	require.NoError(t, db.Create(&p3).Error)
	require.NoError(t, db.Create(&p4).Error)

	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p1.ID, UserID: 1, Tag: "travel"}).Error)
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p2.ID, UserID: 1, Tag: "travel"}).Error)
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p3.ID, UserID: 1, Tag: "travel"}).Error)
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p4.ID, UserID: 2, Tag: "travel"}).Error)

	req := httptest.NewRequest(http.MethodGet, "/remittances/tags/summary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Tags []TagSummaryItem `json:"tags"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Tags, 1)
	assert.Equal(t, "travel", resp.Tags[0].Tag)
	assert.Equal(t, int64(2), resp.Tags[0].PaymentCount)
	assert.Equal(t, []TagAssetTotal{
		{Asset: "USD", Total: 1000},
		{Asset: "XLM", Total: 2500},
	}, resp.Tags[0].Totals)
}

func TestPaymentTags_AutocompleteOrderingWildcardEscapingAndAuth(t *testing.T) {
	db := setupTagTestDB(t)
	r := setupTagRouter(db, 1)

	p1 := models.Payment{SenderID: 1, RecipientID: 2, Amount: 100, Currency: "USD", Status: "completed"}
	p2 := models.Payment{SenderID: 1, RecipientID: 2, Amount: 100, Currency: "USD", Status: "completed"}
	p3 := models.Payment{SenderID: 2, RecipientID: 3, Amount: 100, Currency: "USD", Status: "completed"}
	require.NoError(t, db.Create(&p1).Error)
	require.NoError(t, db.Create(&p2).Error)
	require.NoError(t, db.Create(&p3).Error)

	// "rent_home" used twice, "rent_car" used once, "secret_other" owned by user 2
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p1.ID, UserID: 1, Tag: "rent_home"}).Error)
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p2.ID, UserID: 1, Tag: "rent_home"}).Error)
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p1.ID, UserID: 1, Tag: "rent_car"}).Error)
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p1.ID, UserID: 1, Tag: "rent-other"}).Error)
	require.NoError(t, db.Create(&models.PaymentTag{PaymentID: p3.ID, UserID: 2, Tag: "rent_secret"}).Error)

	// Prefix "RENT_" must escape '_' so "rent-other" is NOT matched, and order by count DESC then alpha
	req := httptest.NewRequest(http.MethodGet, "/remittances/tags/autocomplete?q=RENT_&limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Tags []TagAutocompleteItem `json:"tags"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, []TagAutocompleteItem{
		{Tag: "rent_home", Count: 2},
		{Tag: "rent_car", Count: 1},
	}, resp.Tags)

	// Unauthenticated requests are rejected with 401
	unauthRouter := setupTagRouter(db, 0)
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/remittances?tag=rent_home"},
		{http.MethodPut, "/remittances/1/tags"},
		{http.MethodGet, "/remittances/tags/summary"},
		{http.MethodGet, "/remittances/tags/autocomplete?q=rent"},
	} {
		reqU := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(`{"tags":["a"]}`))
		wU := httptest.NewRecorder()
		unauthRouter.ServeHTTP(wU, reqU)
		assert.Equal(t, http.StatusUnauthorized, wU.Code, "expected 401 for %s %s", tc.method, tc.path)
	}
}
