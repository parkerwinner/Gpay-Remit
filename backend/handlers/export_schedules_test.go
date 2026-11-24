package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func setupExportScheduleTestRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ExportSchedule{}, &models.Payment{}))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set("userID", uint(10))
		c.Next()
	})
	h := NewExportHandler(db)
	r.POST("/export-schedules", h.CreateExportSchedule)
	r.GET("/export-schedules", h.ListExportSchedules)
	r.GET("/export-schedules/:id", h.GetExportSchedule)
	r.PUT("/export-schedules/:id", h.UpdateExportSchedule)
	r.DELETE("/export-schedules/:id", h.DeleteExportSchedule)
	return db, r
}

func TestExportScheduleCRUD(t *testing.T) {
	_, r := setupExportScheduleTestRouter(t)

	// Create
	body, _ := json.Marshal(map[string]string{
		"frequency":   "weekly",
		"format":      "csv",
		"destination": "email",
	})
	req := httptest.NewRequest(http.MethodPost, "/export-schedules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var created models.ExportSchedule
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "weekly", created.Frequency)
	assert.Equal(t, "csv", created.Format)
	assert.Equal(t, "email", created.Destination)
	assert.True(t, created.IsActive)

	// List
	reqList := httptest.NewRequest(http.MethodGet, "/export-schedules", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)
	assert.Equal(t, http.StatusOK, wList.Code)
	var list []models.ExportSchedule
	require.NoError(t, json.Unmarshal(wList.Body.Bytes(), &list))
	assert.Len(t, list, 1)

	// Update
	inactive := false
	updBody, _ := json.Marshal(map[string]interface{}{
		"frequency": "monthly",
		"is_active": inactive,
	})
	reqUpd := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/export-schedules/%d", created.ID), bytes.NewReader(updBody))
	reqUpd.Header.Set("Content-Type", "application/json")
	wUpd := httptest.NewRecorder()
	r.ServeHTTP(wUpd, reqUpd)
	assert.Equal(t, http.StatusOK, wUpd.Code)
	var updated models.ExportSchedule
	require.NoError(t, json.Unmarshal(wUpd.Body.Bytes(), &updated))
	assert.Equal(t, "monthly", updated.Frequency)
	assert.False(t, updated.IsActive)

	// Delete
	reqDel := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/export-schedules/%d", created.ID), nil)
	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, reqDel)
	assert.Equal(t, http.StatusOK, wDel.Code)
}

func TestExportSchedule_RejectsInvalidFrequency(t *testing.T) {
	_, r := setupExportScheduleTestRouter(t)

	body, _ := json.Marshal(map[string]string{
		"frequency":   "hourly",
		"format":      "csv",
		"destination": "email",
	})
	req := httptest.NewRequest(http.MethodPost, "/export-schedules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
