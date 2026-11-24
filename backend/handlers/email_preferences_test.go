package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEmailPreferencesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.EmailPreferences{}))
	return db
}

func setupEmailPreferencesRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set("userID", userID)
		c.Next()
	})
	h := NewEmailPreferencesHandler(db)
	r.GET("/email-preferences", h.GetEmailPreferences)
	r.PUT("/email-preferences", h.UpdateEmailPreferences)
	return r
}

func TestGetEmailPreferences_CreatesDefaults(t *testing.T) {
	db := setupEmailPreferencesTestDB(t)
	r := setupEmailPreferencesRouter(db, 1)

	req := httptest.NewRequest(http.MethodGet, "/email-preferences", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var prefs models.EmailPreferences
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prefs))
	assert.Equal(t, uint(1), prefs.UserID)
	assert.True(t, prefs.TransactionConfirmations)
	assert.False(t, prefs.Marketing)
	assert.True(t, prefs.SecurityAlerts)
}

func TestUpdateEmailPreferences_Success(t *testing.T) {
	db := setupEmailPreferencesTestDB(t)
	r := setupEmailPreferencesRouter(db, 2)

	body, _ := json.Marshal(map[string]bool{
		"transaction_confirmations": false,
		"marketing":                 true,
	})
	req := httptest.NewRequest(http.MethodPut, "/email-preferences", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var prefs models.EmailPreferences
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prefs))
	assert.False(t, prefs.TransactionConfirmations)
	assert.True(t, prefs.Marketing)
	assert.True(t, prefs.SecurityAlerts)
}

func TestUpdateEmailPreferences_RejectsDisablingSecurityAlerts(t *testing.T) {
	db := setupEmailPreferencesTestDB(t)
	r := setupEmailPreferencesRouter(db, 3)

	body, _ := json.Marshal(map[string]bool{
		"security_alerts": false,
	})
	req := httptest.NewRequest(http.MethodPut, "/email-preferences", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
