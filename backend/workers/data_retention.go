package workers

import (
	"context"
	"sync"
	"time"

	"github.com/yourusername/gpay-remit/logger"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

// dataRetentionPeriod is 7 years, per the financial-records retention policy
// referenced in issue #270.
const dataRetentionPeriod = 7 * 365 * 24 * time.Hour

// StartDataRetentionWorker permanently purges soft-deleted audit log rows
// older than the retention period. Payments are intentionally not purged
// here — financial records retention requirements differ from audit-log
// retention and need their own reviewed policy rather than reusing this one.
func StartDataRetentionWorker(ctx context.Context, wg *sync.WaitGroup, db *gorm.DB) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Log.Info("Data retention worker started")

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Log.Info("Data retention worker stopped")
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-dataRetentionPeriod)
				result := db.Unscoped().Where("created_at < ?", cutoff).Delete(&models.AuditLog{})
				if result.Error != nil {
					logger.Log.WithField("error", result.Error).Error("Data retention purge failed")
				} else if result.RowsAffected > 0 {
					logger.Log.WithField("purged_count", result.RowsAffected).Info("Purged expired audit log records")
				}
			}
		}
	}()
}
