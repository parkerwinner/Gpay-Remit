package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/middleware"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupContactTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&models.Contact{}, &models.User{})
	return db
}

func setupContactTestRouter(handler *ContactHandler, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(func(c *gin.Context) {
		if userID > 0 {
			c.Set("userID", userID)
		}
		c.Next()
	})

	router.POST("/contacts", handler.CreateContact)
	router.GET("/contacts", handler.ListContacts)
	router.GET("/contacts/:id", handler.GetContact)
	router.PUT("/contacts/:id", handler.UpdateContact)
	router.DELETE("/contacts/:id", handler.DeleteContact)
	router.POST("/contacts/:id/verify", handler.VerifyContact)

	return router
}

func TestContactHandler_CRUD(t *testing.T) {
	db := setupContactTestDB()
	cfg := &config.Config{}
	mockStellar := &MockStellarClient{
		ValidateAccountFunc: func(accountID string) error {
			if accountID == "GDQJUTQYK2MQX2VGDR2FYWLIYAQIEGXTQVTFEMGH6DNHFMHIDENFINMJFAIL" {
				return fmt.Errorf("account not found")
			}
			return nil
		},
	}

	handler := NewContactHandler(db, cfg, mockStellar)
	router := setupContactTestRouter(handler, 1)

	var contactID uint

	t.Run("Create Contact - Success", func(t *testing.T) {
		body, _ := json.Marshal(CreateContactRequest{
			Nickname:       "Alice Personal",
			StellarAddress: "GBZC6YRFWINCGYH6FFIK3VY4KF3WZJQR7CD3S5Y4GVNIKU5RM3JY7YEX",
			Currency:       "USDC",
			Email:          "alice@example.com",
			Notes:          "Friend in London",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/contacts", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
		var resp models.Contact
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Alice Personal", resp.Nickname)
		assert.Equal(t, models.ContactVerificationVerified, resp.VerificationStatus)
		assert.True(t, resp.IsVerified)
		contactID = resp.ID
	})

	t.Run("Create Contact - Validation Error", func(t *testing.T) {
		body, _ := json.Marshal(CreateContactRequest{
			Nickname:       "",
			StellarAddress: "INVALID",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/contacts", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("List Contacts", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/contacts", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var list []models.Contact
		err := json.Unmarshal(w.Body.Bytes(), &list)
		require.NoError(t, err)
		assert.Len(t, list, 1)
		assert.Equal(t, "Alice Personal", list[0].Nickname)
	})

	t.Run("Get Contact - Success", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/contacts/%d", contactID), nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp models.Contact
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, contactID, resp.ID)
	})

	t.Run("Get Contact - Not Found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/contacts/99999", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Update Contact", func(t *testing.T) {
		body, _ := json.Marshal(UpdateContactRequest{
			Nickname: "Alice Work",
			Notes:    "Updated note",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/contacts/%d", contactID), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp models.Contact
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Alice Work", resp.Nickname)
		assert.Equal(t, "Updated note", resp.Notes)
	})

	t.Run("Verify Contact - Stellar Validation", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/contacts/%d/verify", contactID), nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Delete Contact", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/contacts/%d", contactID), nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Check get after delete returns 404
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/contacts/%d", contactID), nil)
		router.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusNotFound, w2.Code)
	})
}
