package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/yourusername/gpay-remit/logger"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

// Delivery metrics — tracked in-process so the webhook retry worker can
// report retry success/failure counts without pulling in an external
// metrics dependency.
var (
	webhookDeliverySuccessCount atomic.Int64
	webhookDeliveryFailureCount atomic.Int64
)

// WebhookDeliveryMetrics returns the cumulative count of successful and
// failed webhook delivery attempts since process start.
func WebhookDeliveryMetrics() (success int64, failure int64) {
	return webhookDeliverySuccessCount.Load(), webhookDeliveryFailureCount.Load()
}

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

// normalizeAmounts converts numeric amount fields to strings to preserve precision
func normalizeAmounts(data map[string]interface{}) {
	for key, value := range data {
		switch v := value.(type) {
		case float64:
			if key == "amount" || key == "fee" || key == "total" || strings.Contains(key, "amount") || strings.Contains(key, "fee") {
				data[key] = strconv.FormatFloat(v, 'f', -1, 64)
			}
		case map[string]interface{}:
			normalizeAmounts(v)
		}
	}
}

// TriggerWebhook triggers webhooks for a specific event
func (s *WebhookDeliveryService) TriggerWebhook(event string, data map[string]interface{}) error {
	return s.TriggerWebhookWithContext(event, data, "")
}

// TriggerWebhookWithContext triggers webhooks with a request ID for correlation
func (s *WebhookDeliveryService) TriggerWebhookWithContext(event string, data map[string]interface{}, requestID string) error {
	// Find all active webhooks subscribed to this event
	var webhooks []models.Webhook
	if err := s.db.Where("is_active = ?", true).Find(&webhooks).Error; err != nil {
		return fmt.Errorf("failed to fetch webhooks: %w", err)
	}

	// Normalize amounts to strings to preserve precision
	normalizeAmounts(data)

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
		go s.DeliverWebhook(&webhook, &delivery, requestID)
	}

	return nil
}

// isConnectionRefused checks if the error is a connection refused error
func isConnectionRefused(opErr *net.OpError) bool {
	if opErr.Err == nil {
		return false
	}
	// Check for syscall.ECONNREFUSED in the error chain
	return errors.Is(opErr.Err, syscall.ECONNREFUSED)
}

// ErrorClassification represents different types of webhook delivery errors
type ErrorClassification int

const (
	ErrorTimeout ErrorClassification = iota
	ErrorDNS
	ErrorConnectionRefused
	ErrorHTTP4xx
	ErrorHTTP5xx
	ErrorOther
)

// classifyError categorizes the error for appropriate retry handling (#197)
func classifyError(err error, statusCode int) ErrorClassification {
	if err == nil && statusCode >= 400 {
		if statusCode >= 400 && statusCode < 500 {
			return ErrorHTTP4xx
		}
		if statusCode >= 500 {
			return ErrorHTTP5xx
		}
	}

	var netErr net.Error
	var dnsErr *net.DNSError
	var opErr *net.OpError

	switch {
	case errors.As(err, &dnsErr):
		return ErrorDNS
	case errors.As(err, &netErr) && netErr.Timeout():
		return ErrorTimeout
	case errors.As(err, &opErr) && isConnectionRefused(opErr):
		return ErrorConnectionRefused
	default:
		return ErrorOther
	}
}

// getRetryPolicy returns retry configuration based on error type (#197)
func getRetryPolicy(errorType ErrorClassification) (maxRetries int, baseDelay time.Duration, shouldRetry bool) {
	switch errorType {
	case ErrorTimeout:
		// Timeouts indicate transient network congestion - retry with exponential backoff
		return 3, time.Second, true
	case ErrorDNS:
		// DNS failures may be transient but aggressive retrying is counterproductive
		return 2, 5 * time.Second, true
	case ErrorConnectionRefused:
		// Connection refused typically indicates server down - don't retry immediately
		return 0, 30 * time.Second, false // Mark as failed, schedule for later
	case ErrorHTTP5xx:
		// Server errors - retry with exponential backoff
		return 3, time.Second, true
	case ErrorHTTP4xx:
		// Client errors - malformed request, don't retry
		return 0, 0, false
	case ErrorOther:
		// Unclassified errors - single retry with moderate delay
		return 1, 2 * time.Second, true
	default:
		return 1, 2 * time.Second, true
	}
}
// DeliverWebhook delivers a webhook with per-error-type retry logic (#197)
func (s *WebhookDeliveryService) DeliverWebhook(webhook *models.Webhook, delivery *models.WebhookDelivery, requestID string) {
	for {
		delivery.AttemptCount++
		
		success, responseCode, responseBody, errMsg, err := s.sendWebhookRequest(webhook, delivery.Payload, requestID)
		
		delivery.ResponseCode = responseCode
		delivery.ResponseBody = responseBody
		delivery.ErrorMessage = errMsg

		if success {
			delivery.Status = "success"
			now := time.Now()
			delivery.CompletedAt = &now
			delivery.NextRetryAt = nil
			s.db.Save(delivery)
			webhookDeliverySuccessCount.Add(1)

			logger.Log.WithField("webhook_id", webhook.ID).
				WithField("delivery_id", delivery.ID).
				Info("Webhook delivered successfully")
			return
		}

		// Classify error type for appropriate retry handling
		errorType := classifyError(err, responseCode)
		maxRetries, baseDelay, shouldRetry := getRetryPolicy(errorType)

		// Log the error with classification
		logLevel := logger.Log.WithField("webhook_id", webhook.ID).
			WithField("delivery_id", delivery.ID).
			WithField("attempt", delivery.AttemptCount).
			WithField("error_type", fmt.Sprintf("%d", errorType))

		if err != nil {
			logLevel = logLevel.WithError(err)
		}

		if !shouldRetry || delivery.AttemptCount >= maxRetries {
			// Mark as failed
			delivery.Status = "failed"
			now := time.Now()
			delivery.CompletedAt = &now
			delivery.NextRetryAt = nil
			s.db.Save(delivery)
			webhookDeliveryFailureCount.Add(1)

			logLevel.Error("Webhook delivery permanently failed")
			return
		}

		// Calculate delay based on error type and attempt count
		var delay time.Duration
		if errorType == ErrorTimeout || errorType == ErrorHTTP5xx {
			// Exponential backoff for timeout and 5xx errors
			delay = baseDelay * time.Duration(1<<uint(delivery.AttemptCount-1))
		} else {
			// Fixed delay for other retryable errors
			delay = baseDelay
		}

		// Schedule next retry
		nextRetry := time.Now().Add(delay)
		delivery.NextRetryAt = &nextRetry
		s.db.Save(delivery)

		logLevel.Warn("Webhook delivery failed, will retry after delay")
		
		// Wait before retry
		time.Sleep(delay)
	}
}

// sendWebhookRequest sends the HTTP request to the webhook URL
func (s *WebhookDeliveryService) sendWebhookRequest(webhook *models.Webhook, payload string, requestID string) (success bool, responseCode int, responseBody string, errorMsg string, err error) {
	// Create signature
	signature := s.generateSignature(webhook.Secret, payload)

	req, err := http.NewRequest("POST", webhook.URL, bytes.NewBufferString(payload))
	if err != nil {
		return false, 0, "", fmt.Sprintf("failed to create request: %v", err), err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)
	req.Header.Set("X-Webhook-ID", fmt.Sprintf("%d", webhook.ID))
	req.Header.Set("User-Agent", "GPay-Remit-Webhook/1.0")
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, 0, "", fmt.Sprintf("request failed: %v", err), err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, resp.StatusCode, "", fmt.Sprintf("failed to read response: %v", err), err
	}

	responseBody = string(body)
	if len(responseBody) > 1000 {
		responseBody = responseBody[:1000] + "... (truncated)"
	}

	// Consider 2xx status codes as success
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, resp.StatusCode, responseBody, "", nil
	}

	return false, resp.StatusCode, responseBody, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, responseBody), nil
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

		go s.DeliverWebhook(&webhook, &delivery, "")
	}

	return nil
}
