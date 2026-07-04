package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yourusername/gpay-remit/logger"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

type WebhookDeliveryService struct {
	db         *gorm.DB
	httpClient *http.Client
}

type WebhookPayload struct {
	Event     string                 `json:"event"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

func NewWebhookDeliveryService(db *gorm.DB) *WebhookDeliveryService {
	return &WebhookDeliveryService{
		db: db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TriggerWebhook triggers webhooks for a specific event
func (s *WebhookDeliveryService) TriggerWebhook(event string, data map[string]interface{}) error {
	// Find all active webhooks subscribed to this event
	var webhooks []models.Webhook
	if err := s.db.Where("is_active = ?", true).Find(&webhooks).Error; err != nil {
		return fmt.Errorf("failed to fetch webhooks: %w", err)
	}

	for _, webhook := range webhooks {
		// Check if webhook is subscribed to this event
		events := strings.Split(webhook.Events, ",")
		subscribed := false
		for _, e := range events {
			if strings.TrimSpace(e) == event || strings.TrimSpace(e) == "*" {
				subscribed = true
				break
			}
		}

		if !subscribed {
			continue
		}

		// Create webhook delivery record
		payload := WebhookPayload{
			Event:     event,
			Timestamp: time.Now(),
			Data:      data,
		}

		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			logger.Log.WithField("webhook_id", webhook.ID).WithError(err).Error("Failed to marshal webhook payload")
			continue
		}

		delivery := models.WebhookDelivery{
			WebhookID:    webhook.ID,
			Event:        event,
			Payload:      string(payloadJSON),
			Status:       "pending",
			AttemptCount: 0,
		}

		if err := s.db.Create(&delivery).Error; err != nil {
			logger.Log.WithField("webhook_id", webhook.ID).WithError(err).Error("Failed to create webhook delivery")
			continue
		}

		// Deliver asynchronously
		go s.DeliverWebhook(&webhook, &delivery)
	}

	return nil
}

// DeliverWebhook delivers a webhook with retry logic
func (s *WebhookDeliveryService) DeliverWebhook(webhook *models.Webhook, delivery *models.WebhookDelivery) {
	maxAttempts := 5
	baseDelay := time.Second

	for attempt := 0; attempt < maxAttempts; attempt++ {
		delivery.AttemptCount = attempt + 1
		
		// Exponential backoff
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<uint(attempt-1)) // 1s, 2s, 4s, 8s, 16s
			time.Sleep(delay)
		}

		success, responseCode, responseBody, errMsg := s.sendWebhookRequest(webhook, delivery.Payload)

		delivery.ResponseCode = responseCode
		delivery.ResponseBody = responseBody
		delivery.ErrorMessage = errMsg

		if success {
			delivery.Status = "success"
			now := time.Now()
			delivery.CompletedAt = &now
			delivery.NextRetryAt = nil
			s.db.Save(delivery)
			
			logger.Log.WithField("webhook_id", webhook.ID).
				WithField("delivery_id", delivery.ID).
				Info("Webhook delivered successfully")
			return
		}

		// Calculate next retry time
		if attempt < maxAttempts-1 {
			nextDelay := baseDelay * time.Duration(1<<uint(attempt))
			nextRetry := time.Now().Add(nextDelay)
			delivery.NextRetryAt = &nextRetry
		}

		s.db.Save(delivery)
		
		logger.Log.WithField("webhook_id", webhook.ID).
			WithField("delivery_id", delivery.ID).
			WithField("attempt", attempt+1).
			WithError(fmt.Errorf("%s", errMsg)).
			Warn("Webhook delivery failed, will retry")
	}

	// All attempts failed
	delivery.Status = "failed"
	now := time.Now()
	delivery.CompletedAt = &now
	delivery.NextRetryAt = nil
	s.db.Save(delivery)
	
	logger.Log.WithField("webhook_id", webhook.ID).
		WithField("delivery_id", delivery.ID).
		Error("Webhook delivery failed after all retry attempts")
}

// sendWebhookRequest sends the HTTP request to the webhook URL
func (s *WebhookDeliveryService) sendWebhookRequest(webhook *models.Webhook, payload string) (success bool, responseCode int, responseBody string, errorMsg string) {
	// Create signature
	signature := s.generateSignature(webhook.Secret, payload)

	req, err := http.NewRequest("POST", webhook.URL, bytes.NewBufferString(payload))
	if err != nil {
		return false, 0, "", fmt.Sprintf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)
	req.Header.Set("X-Webhook-ID", fmt.Sprintf("%d", webhook.ID))
	req.Header.Set("User-Agent", "GPay-Remit-Webhook/1.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, 0, "", fmt.Sprintf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, resp.StatusCode, "", fmt.Sprintf("failed to read response: %v", err)
	}

	responseBody = string(body)
	if len(responseBody) > 1000 {
		responseBody = responseBody[:1000] + "... (truncated)"
	}

	// Consider 2xx status codes as success
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, resp.StatusCode, responseBody, ""
	}

	return false, resp.StatusCode, responseBody, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, responseBody)
}

// generateSignature creates HMAC-SHA256 signature for webhook verification
func (s *WebhookDeliveryService) generateSignature(secret, payload string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifySignature verifies the webhook signature
func VerifySignature(secret, payload, signature string) bool {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	expectedSignature := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}

// RetryFailedDeliveries retries webhook deliveries that are pending or failed
func (s *WebhookDeliveryService) RetryFailedDeliveries() error {
	var deliveries []models.WebhookDelivery
	now := time.Now()
	
	// Find deliveries that need retry
	if err := s.db.Where("status IN (?) AND (next_retry_at IS NULL OR next_retry_at <= ?)", 
		[]string{"pending", "failed"}, now).
		Where("attempt_count < ?", 5).
		Find(&deliveries).Error; err != nil {
		return fmt.Errorf("failed to fetch failed deliveries: %w", err)
	}

	for _, delivery := range deliveries {
		var webhook models.Webhook
		if err := s.db.First(&webhook, delivery.WebhookID).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch webhook for retry")
			continue
		}

		if !webhook.IsActive {
			continue
		}

		go s.DeliverWebhook(&webhook, &delivery)
	}

	return nil
}
