package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRecurringTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Payment{}, &models.RecurringSchedule{}, &models.RecurringScheduleExecution{}))
	return db
}

func setupRecurringRouter(db *gorm.DB, authedUserID *uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewRecurringScheduleHandler(db, &config.Config{}, &MockStellarClient{})
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	if authedUserID != nil {
		uid := *authedUserID
		r.Use(func(c *gin.Context) {
			c.Set("userID", uid)
			c.Next()
		})
	}
	r.POST("/recurring-schedules", h.CreateSchedule)
	r.GET("/recurring-schedules", h.ListSchedules)
	r.GET("/recurring-schedules/:id", h.GetSchedule)
	r.POST("/recurring-schedules/:id/pause", h.PauseSchedule)
	r.POST("/recurring-schedules/:id/resume", h.ResumeSchedule)
	r.POST("/recurring-schedules/:id/cancel", h.CancelSchedule)
	return r
}

func TestRecurringScheduleLifecycleAndOwnership(t *testing.T) {
	t.Run("Create, pause, resume moves next_run_at to future, and cancel", func(t *testing.T) {
		db := setupRecurringTestDB(t)
		u1, _ := seedTemplateUsers(t, db)
		uid := u1.ID
		router := setupRecurringRouter(db, &uid)

		body, _ := json.Marshal(CreateRecurringScheduleRequest{
			SenderAccount:    testSenderAddr,
			RecipientAccount: testRecipientAddr1,
			Amount:           50000000,
			Currency:         "USDC",
			Frequency:        "monthly",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/recurring-schedules", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		var created models.RecurringSchedule
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
		assert.Equal(t, models.RecurringStatusActive, created.Status)

		// Pause
		wPause := httptest.NewRecorder()
		reqPause, _ := http.NewRequest("POST", fmt.Sprintf("/recurring-schedules/%d/pause", created.ID), nil)
		router.ServeHTTP(wPause, reqPause)
		require.Equal(t, http.StatusOK, wPause.Code)
		assert.Contains(t, wPause.Body.String(), models.RecurringStatusPaused)

		// Simulate schedule having been paused in the past
		past := time.Now().UTC().Add(-48 * time.Hour)
		require.NoError(t, db.Model(&created).Update("next_run_at", past).Error)

		// Resume moves next_run_at to a future UTC time
		wResume := httptest.NewRecorder()
		reqResume, _ := http.NewRequest("POST", fmt.Sprintf("/recurring-schedules/%d/resume", created.ID), nil)
		router.ServeHTTP(wResume, reqResume)
		require.Equal(t, http.StatusOK, wResume.Code)
		var resumed models.RecurringSchedule
		require.NoError(t, json.Unmarshal(wResume.Body.Bytes(), &resumed))
		assert.Equal(t, models.RecurringStatusActive, resumed.Status)
		assert.True(t, resumed.NextRunAt.After(time.Now().UTC()))

		// Cancel
		wCancel := httptest.NewRecorder()
		reqCancel, _ := http.NewRequest("POST", fmt.Sprintf("/recurring-schedules/%d/cancel", created.ID), nil)
		router.ServeHTTP(wCancel, reqCancel)
		require.Equal(t, http.StatusOK, wCancel.Code)
		assert.Contains(t, wCancel.Body.String(), models.RecurringStatusCancelled)

		// Cannot pause or resume cancelled schedule
		wPauseAfterCancel := httptest.NewRecorder()
		reqPAC, _ := http.NewRequest("POST", fmt.Sprintf("/recurring-schedules/%d/pause", created.ID), nil)
		router.ServeHTTP(wPauseAfterCancel, reqPAC)
		assert.Equal(t, http.StatusBadRequest, wPauseAfterCancel.Code)

		wResumeAfterCancel := httptest.NewRecorder()
		reqRAC, _ := http.NewRequest("POST", fmt.Sprintf("/recurring-schedules/%d/resume", created.ID), nil)
		router.ServeHTTP(wResumeAfterCancel, reqRAC)
		assert.Equal(t, http.StatusBadRequest, wResumeAfterCancel.Code)
	})

	t.Run("Ownership enforced on all endpoints", func(t *testing.T) {
		db := setupRecurringTestDB(t)
		u1, u2 := seedTemplateUsers(t, db)
		sched := models.RecurringSchedule{
			UserID:           u1.ID,
			SenderAccount:    testSenderAddr,
			RecipientAccount: testRecipientAddr1,
			Amount:           1000,
			Currency:         "USDC",
			Frequency:        "daily",
			Status:           models.RecurringStatusActive,
			NextRunAt:        time.Now().UTC().Add(24 * time.Hour),
		}
		require.NoError(t, db.Create(&sched).Error)

		uid2 := u2.ID
		router := setupRecurringRouter(db, &uid2)

		for _, tc := range []struct{ method, path string }{
			{"GET", fmt.Sprintf("/recurring-schedules/%d", sched.ID)},
			{"POST", fmt.Sprintf("/recurring-schedules/%d/pause", sched.ID)},
			{"POST", fmt.Sprintf("/recurring-schedules/%d/resume", sched.ID)},
			{"POST", fmt.Sprintf("/recurring-schedules/%d/cancel", sched.ID)},
		} {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tc.method, tc.path, nil)
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusForbidden, w.Code)
		}
	})
}
