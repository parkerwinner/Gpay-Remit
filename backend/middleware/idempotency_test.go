package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStartIdempotencyCleanupSchedulerCleansExpiredRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.IdempotencyRecord{}))

	now := time.Now()
	expiredRecord := models.IdempotencyRecord{
		IdempotencyKey: "expired-key",
		RequestHash:    "expired-hash",
		RequestMethod:  "POST",
		RequestPath:    "/payments",
		Status:         "completed",
		ExpiresAt:      now.Add(-time.Hour),
	}
	activeRecord := models.IdempotencyRecord{
		IdempotencyKey: "active-key",
		RequestHash:    "active-hash",
		RequestMethod:  "POST",
		RequestPath:    "/payments",
		Status:         "completed",
		ExpiresAt:      now.Add(time.Hour),
	}

	require.NoError(t, db.Create(&expiredRecord).Error)
	require.NoError(t, db.Create(&activeRecord).Error)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartIdempotencyCleanupScheduler(ctx, db, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		var count int64
		err := db.Model(&models.IdempotencyRecord{}).Count(&count).Error
		return err == nil && count == 1
	}, time.Second, 10*time.Millisecond)

	var remaining []models.IdempotencyRecord
	require.NoError(t, db.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, "active-key", remaining[0].IdempotencyKey)
}
