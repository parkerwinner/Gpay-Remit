package workers

import (
	"context"
	"sync"
	"time"

	"github.com/yourusername/gpay-remit/logger"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

func StartPaymentRequestExpiryWorker(ctx context.Context, wg *sync.WaitGroup, db *gorm.DB) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Log.Info("Payment request expiry worker started")

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Log.Info("Payment request expiry worker stopped")
				return
			case <-ticker.C:
				count, err := models.ExpireStalePaymentRequests(db)
				if err != nil {
					logger.Log.WithField("error", err).Error("Failed to expire stale payment requests")
				} else if count > 0 {
					logger.Log.WithField("expired_count", count).Info("Expired stale payment requests")
				}
			}
		}
	}()
}
