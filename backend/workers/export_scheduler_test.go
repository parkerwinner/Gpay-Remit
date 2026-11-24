package workers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProcessDueExportSchedules_WebhookDelivery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Payment{}, &models.ExportSchedule{}))

	user := models.User{
		Email:          "export@example.com",
		Name:           "Export User",
		StellarAddress: "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
		PasswordHash:   "hash",
	}
	require.NoError(t, db.Create(&user).Error)

	payment := models.Payment{
		SenderID:         user.ID,
		RecipientID:      2,
		SenderAccount:    user.StellarAddress,
		RecipientAccount: "GBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
		Amount:           15000,
		Currency:         "USD",
		Status:           "completed",
	}
	require.NoError(t, db.Create(&payment).Error)

	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	now := time.Now().UTC()
	sched := models.ExportSchedule{
		UserID:      user.ID,
		Frequency:   "daily",
		Format:      "csv",
		Destination: "webhook",
		WebhookURL:  server.URL,
		IsActive:    true,
		NextRunAt:   now.Add(-5 * time.Minute),
	}
	require.NoError(t, db.Create(&sched).Error)

	emailSvc := services.NewEmailService("", "", "", "", "", false)
	processed, err := ProcessDueExportSchedules(context.Background(), db, emailSvc, nil, server.Client(), now)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, "export.completed", receivedPayload["event"])
	assert.Equal(t, float64(1), receivedPayload["row_count"])

	var reloaded models.ExportSchedule
	require.NoError(t, db.First(&reloaded, sched.ID).Error)
	assert.Equal(t, "success", reloaded.LastStatus)
	assert.True(t, reloaded.NextRunAt.After(now))
}
