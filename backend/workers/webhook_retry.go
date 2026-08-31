package workers

import (
	"context"
	"sync"
	"time"

	"github.com/yourusername/gpay-remit/logger"
	"github.com/yourusername/gpay-remit/services"
	"gorm.io/gorm"
)

const webhookRetryInterval = 5 * time.Minute

// StartWebhookRetryWorker periodically retries failed/pending webhook
// deliveries so they are not stuck forever after a transient failure.
func StartWebhookRetryWorker(ctx context.Context, wg *sync.WaitGroup, db *gorm.DB) {
	deliveryService := services.NewWebhookDeliveryService(db)

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Log.WithField("interval", webhookRetryInterval.String()).Info("Webhook retry worker started")

		ticker := time.NewTicker(webhookRetryInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Log.Info("Webhook retry worker stopped")
				return
			case <-ticker.C:
				runWebhookRetryCycle(deliveryService)
			}
		}
	}()
}

func runWebhookRetryCycle(deliveryService *services.WebhookDeliveryService) {
	if err := deliveryService.RetryFailedDeliveries(); err != nil {
		logger.Log.WithField("error", err).Error("Webhook retry cycle failed")
		return
	}

	successCount, failureCount := services.WebhookDeliveryMetrics()
	logger.Log.WithField("cumulative_success", successCount).
		WithField("cumulative_failure", failureCount).
		Info("Webhook retry cycle completed")
}
