package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTemplateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Payment{}, &models.PaymentTemplate{}))
	return db
}

func setupTemplateRouter(db *gorm.DB, stellar *MockStellarClient, authedUserID *uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{PlatformFeeBps: 50}
	h := NewPaymentTemplateHandler(db, cfg, stellar)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	if authedUserID != nil {
		uid := *authedUserID
		r.Use(func(c *gin.Context) {
			c.Set("userID", uid)
			c.Next()
		})
	}
	r.POST("/templates", h.CreateTemplate)
	r.GET("/templates", h.ListTemplates)
	r.GET("/templates/:id", h.GetTemplate)
	r.PUT("/templates/:id", h.UpdateTemplate)
	r.DELETE("/templates/:id", h.DeleteTemplate)
	r.POST("/templates/:id/send", h.SendFromTemplate)
	return r
}

func seedTemplateUsers(t *testing.T, db *gorm.DB) (models.User, models.User) {
	t.Helper()
	u1 := models.User{ID: 1, Email: "u1@example.com", Name: "User One", StellarAddress: testSenderAddr, PasswordHash: "h", IsActive: true}
	u2 := models.User{ID: 2, Email: "u2@example.com", Name: "User Two", StellarAddress: testOtherAddr, PasswordHash: "h", IsActive: true}
	require.NoError(t, db.Create(&u1).Error)
	require.NoError(t, db.Create(&u2).Error)
	return u1, u2
}

func TestPaymentTemplatesCRUDAndSend(t *testing.T) {
	t.Run("CRUD success and send from template", func(t *testing.T) {
		db := setupTemplateTestDB(t)
		u1, _ := seedTemplateUsers(t, db)
		uid := u1.ID
		stellar := &MockStellarClient{ValidateAccountFunc: func(accountID string) error { return nil }}
		router := setupTemplateRouter(db, stellar, &uid)

		// Create
		createBody, _ := json.Marshal(CreatePaymentTemplateRequest{
			TemplateName:     "Monthly Rent",
			RecipientAccount: testRecipientAddr1,
			Amount:           125.50,
			AssetCode:        "USDC",
			Notes:            "Apartment rent",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/templates", bytes.NewBuffer(createBody))
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		var created models.PaymentTemplate
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
		assert.Equal(t, "Monthly Rent", created.TemplateName)
		assert.Equal(t, u1.ID, created.UserID)

		// List
		wList := httptest.NewRecorder()
		reqList, _ := http.NewRequest("GET", "/templates?page=1&page_size=10", nil)
		router.ServeHTTP(wList, reqList)
		assert.Equal(t, http.StatusOK, wList.Code)
		assert.Contains(t, wList.Body.String(), "Monthly Rent")

		// Get
		wGet := httptest.NewRecorder()
		reqGet, _ := http.NewRequest("GET", fmt.Sprintf("/templates/%d", created.ID), nil)
		router.ServeHTTP(wGet, reqGet)
		assert.Equal(t, http.StatusOK, wGet.Code)

		// Update
		updateBody, _ := json.Marshal(UpdatePaymentTemplateRequest{TemplateName: "Updated Rent", Amount: 150})
		wUpd := httptest.NewRecorder()
		reqUpd, _ := http.NewRequest("PUT", fmt.Sprintf("/templates/%d", created.ID), bytes.NewBuffer(updateBody))
		router.ServeHTTP(wUpd, reqUpd)
		assert.Equal(t, http.StatusOK, wUpd.Code)
		assert.Contains(t, wUpd.Body.String(), "Updated Rent")

		// Send from template
		wSend := httptest.NewRecorder()
		reqSend, _ := http.NewRequest("POST", fmt.Sprintf("/templates/%d/send", created.ID), nil)
		router.ServeHTTP(wSend, reqSend)
		require.Equal(t, http.StatusCreated, wSend.Code)

		var payment models.Payment
		require.NoError(t, db.First(&payment).Error)
		assert.Equal(t, u1.ID, payment.SenderID)
		assert.Equal(t, testRecipientAddr1, payment.RecipientAccount)
		assert.Equal(t, int64(1500000000), payment.Amount)
		assert.Equal(t, "USDC", payment.Currency)
		assert.Equal(t, "pending", payment.Status)

		// Delete
		wDel := httptest.NewRecorder()
		reqDel, _ := http.NewRequest("DELETE", fmt.Sprintf("/templates/%d", created.ID), nil)
		router.ServeHTTP(wDel, reqDel)
		assert.Equal(t, http.StatusOK, wDel.Code)

		// Deleted template cannot be sent
		wSendDeleted := httptest.NewRecorder()
		reqSendDeleted, _ := http.NewRequest("POST", fmt.Sprintf("/templates/%d/send", created.ID), nil)
		router.ServeHTTP(wSendDeleted, reqSendDeleted)
		assert.Equal(t, http.StatusNotFound, wSendDeleted.Code)
	})

	t.Run("Validation failure", func(t *testing.T) {
		db := setupTemplateTestDB(t)
		u1, _ := seedTemplateUsers(t, db)
		uid := u1.ID
		stellar := &MockStellarClient{}
		router := setupTemplateRouter(db, stellar, &uid)

		body, _ := json.Marshal(CreatePaymentTemplateRequest{
			TemplateName:     "Bad",
			RecipientAccount: "INVALID",
			Amount:           10,
			AssetCode:        "ZZZ",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/templates", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Not found and another user forbidden", func(t *testing.T) {
		db := setupTemplateTestDB(t)
		u1, u2 := seedTemplateUsers(t, db)
		tpl := models.PaymentTemplate{
			UserID:           u1.ID,
			TemplateName:     "U1 Template",
			RecipientAccount: testRecipientAddr1,
			Amount:           25,
			AssetCode:        "USD",
		}
		require.NoError(t, db.Create(&tpl).Error)

		uid2 := u2.ID
		stellar := &MockStellarClient{}
		router := setupTemplateRouter(db, stellar, &uid2)

		// Another user's template -> 403
		for _, tc := range []struct{ method, path string }{
			{"GET", fmt.Sprintf("/templates/%d", tpl.ID)},
			{"PUT", fmt.Sprintf("/templates/%d", tpl.ID)},
			{"DELETE", fmt.Sprintf("/templates/%d", tpl.ID)},
			{"POST", fmt.Sprintf("/templates/%d/send", tpl.ID)},
		} {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tc.method, tc.path, bytes.NewBufferString("{}"))
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusForbidden, w.Code)
		}

		// List does not leak other user's templates
		wList := httptest.NewRecorder()
		reqList, _ := http.NewRequest("GET", "/templates", nil)
		router.ServeHTTP(wList, reqList)
		assert.Equal(t, http.StatusOK, wList.Code)
		assert.NotContains(t, wList.Body.String(), "U1 Template")

		// Non-existent -> 404
		wNF := httptest.NewRecorder()
		reqNF, _ := http.NewRequest("GET", "/templates/9999", nil)
		router.ServeHTTP(wNF, reqNF)
		assert.Equal(t, http.StatusNotFound, wNF.Code)
	})

	t.Run("Sending template with now-invalid recipient fails", func(t *testing.T) {
		db := setupTemplateTestDB(t)
		u1, _ := seedTemplateUsers(t, db)
		tpl := models.PaymentTemplate{
			UserID:           u1.ID,
			TemplateName:     "Blocked later",
			RecipientAccount: testRecipientAddr1,
			Amount:           25,
			AssetCode:        "USD",
		}
		require.NoError(t, db.Create(&tpl).Error)

		uid := u1.ID
		stellar := &MockStellarClient{
			ValidateAccountFunc: func(accountID string) error {
				return errors.New("account no longer exists")
			},
		}
		router := setupTemplateRouter(db, stellar, &uid)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", fmt.Sprintf("/templates/%d/send", tpl.ID), nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Unauthenticated returns 401", func(t *testing.T) {
		db := setupTemplateTestDB(t)
		router := setupTemplateRouter(db, &MockStellarClient{}, nil)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/templates", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
