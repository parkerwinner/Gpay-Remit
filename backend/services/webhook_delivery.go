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

	"github.com/Gpay-Remit/gpay-remit/logger"
	"github.com/Gpay-Remit/gpay-remit/metrics"
	"github.com/Gpay-Remit/gpay-remit/models"
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
			MaxRetries:   models.DefaultWebhookMaxRetries,
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

// DeliverWebhook performs the initial delivery of a webhook. Transient
// failures get a few short in-line retries (see getRetryPolicy). When those
// are used up, the delivery is handed to the retry worker, which follows the
// WebhookRetryBackoff schedule.
func (s *WebhookDeliveryService) DeliverWebhook(webhook *models.Webhook, delivery *models.WebhookDelivery, requestID string) {
	s.deliver(webhook, delivery, requestID, true)
}

// deliver makes attempts at delivering a webhook.
//
// allowInline is true for the initial delivery and false for worker-driven
// retries. A worker retry makes exactly one attempt, so each step of the
// backoff schedule produces a single request rather than a burst of them.
func (s *WebhookDeliveryService) deliver(webhook *models.Webhook, delivery *models.WebhookDelivery, requestID string, allowInline bool) {
	kind := "scheduled"
	if allowInline {
		kind = "inline"
	}

	for {
		delivery.AttemptCount++
		isRetry := delivery.AttemptCount > 1

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
			if isRetry {
				metrics.WebhookRetryAttemptsTotal.WithLabelValues(kind, "success").Inc()
			}

			logger.Log.WithField("webhook_id", webhook.ID).
				WithField("delivery_id", delivery.ID).
				Info("Webhook delivered successfully")
			return
		}

		if isRetry {
			metrics.WebhookRetryAttemptsTotal.WithLabelValues(kind, "failure").Inc()
		}

		// Classify error type for appropriate retry handling
		errorType := classifyError(err, responseCode)
		maxInline, baseDelay, _ := getRetryPolicy(errorType)

		// Log the error with classification
		logLevel := logger.Log.WithField("webhook_id", webhook.ID).
			WithField("delivery_id", delivery.ID).
			WithField("attempt", delivery.AttemptCount).
			WithField("error_type", fmt.Sprintf("%d", errorType))

		if err != nil {
			logLevel = logLevel.WithError(err)
		}

		// A 4xx means the endpoint rejected this request. Sending the same
		// payload again will not change that, so it is dead-lettered now.
		if errorType == ErrorHTTP4xx {
			s.markDeadLetter(delivery, "client_error")
			logLevel.Error("Webhook delivery permanently failed, moved to dead-letter queue")
			return
		}

		// Transient failure: retry in-line while the initial delivery still has
		// in-line budget. Timeouts and 5xx back off exponentially, the rest use a fixed delay.
		if allowInline && delivery.AttemptCount < maxInline {
			delay := baseDelay
			if errorType == ErrorTimeout || errorType == ErrorHTTP5xx {
				delay = baseDelay * time.Duration(1<<uint(delivery.AttemptCount-1))
			}
			logLevel.WithField("retry_in", delay.String()).Warn("Webhook delivery failed, retrying in-line")
			time.Sleep(delay)
			continue
		}

		// In-line budget used up: move to the scheduled backoff, or to the dead-letter queue.
		if next, delay, deadLettered := s.scheduleOrDeadLetter(delivery); deadLettered {
			logLevel.Error("Webhook delivery retries exhausted, moved to dead-letter queue")
		} else {
			logLevel.WithField("retry_in", delay.String()).
				WithField("next_retry_at", next.UTC().Format(time.RFC3339)).
				Warn("Webhook delivery failed, retry scheduled")
		}
		return
	}
}

// scheduleOrDeadLetter moves a transiently failed delivery to its next backoff
// step. It returns deadLettered=true instead when the delivery has used all
// MaxRetries scheduled retries.
func (s *WebhookDeliveryService) scheduleOrDeadLetter(delivery *models.WebhookDelivery) (next time.Time, delay time.Duration, deadLettered bool) {
	maxRetries := delivery.MaxRetries
	if maxRetries <= 0 {
		maxRetries = models.DefaultWebhookMaxRetries
	}
	if delivery.RetryCount >= maxRetries {
		s.markDeadLetter(delivery, "retries_exhausted")
		return time.Time{}, 0, true
	}

	step := delivery.RetryCount
	if step >= len(models.WebhookRetryBackoff) {
		step = len(models.WebhookRetryBackoff) - 1
	}
	delay = models.WebhookRetryBackoff[step]
	delivery.RetryCount++

	next = time.Now().Add(delay)
	delivery.Status = "failed"
	delivery.NextRetryAt = &next
	delivery.CompletedAt = nil
	s.db.Save(delivery)
	return next, delay, false
}

// markDeadLetter records a permanent failure: the delivery is no longer retried
// and is listed by ListDeadLetterDeliveries.
func (s *WebhookDeliveryService) markDeadLetter(delivery *models.WebhookDelivery, reason string) {
	now := time.Now()
	delivery.Status = "failed"
	delivery.CompletedAt = &now
	delivery.NextRetryAt = nil
	delivery.DeadLetteredAt = &now
	s.db.Save(delivery)

	webhookDeliveryFailureCount.Add(1)
	metrics.WebhookDeadLettersTotal.WithLabelValues(reason).Inc()
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
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
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

// VerifyWebhookDelivery verifies an incoming webhook delivery on the consumer side.
// It verifies:
// 1. Signature header is present.
// 2. Signature matches the HMAC-SHA256 of the payload.
// 3. Timestamp header is present, parseable, and not expired (within maxAge tolerance).
func VerifyWebhookDelivery(secret string, headers http.Header, payload []byte, maxAge time.Duration) error {
	sig := headers.Get("X-Webhook-Signature")
	if sig == "" {
		return errors.New("missing X-Webhook-Signature header")
	}

	if !VerifySignature(secret, string(payload), sig) {
		return errors.New("invalid webhook signature: payload does not match HMAC signature")
	}

	if maxAge > 0 {
		tsStr := headers.Get("X-Webhook-Timestamp")
		if tsStr == "" {
			return errors.New("missing X-Webhook-Timestamp header")
		}
		tsUnix, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid X-Webhook-Timestamp format: %w", err)
		}
		sentAt := time.Unix(tsUnix, 0)
		age := time.Since(sentAt)
		if age < 0 {
			age = -age
		}
		if age > maxAge {
			return fmt.Errorf("webhook timestamp expired: age %v exceeds max tolerance %v", age, maxAge)
		}
	}

	return nil
}

// retryClaimLease stops the next worker tick from picking up a delivery whose
// scheduled attempt is still in flight. Its outcome overwrites the lease.
const retryClaimLease = 2 * time.Minute

// RetryFailedDeliveries runs one pass of the retry worker. It makes one attempt
// for each delivery that is due: pending deliveries never attempted, and failed
// deliveries whose backoff has elapsed and that still have retries left.
// Dead-lettered deliveries are never selected.
func (s *WebhookDeliveryService) RetryFailedDeliveries() error {
	deliveries, err := dueWebhookDeliveries(s.db, time.Now())
	if err != nil {
		return fmt.Errorf("failed to fetch failed deliveries: %w", err)
	}

	for i := range deliveries {
		delivery := &deliveries[i]

		var webhook models.Webhook
		if err := s.db.First(&webhook, delivery.WebhookID).Error; err != nil {
			logger.Log.WithError(err).Error("Failed to fetch webhook for retry")
			continue
		}

		if !webhook.IsActive {
			continue
		}

		s.db.Model(delivery).Update("next_retry_at", time.Now().Add(retryClaimLease))
		go s.deliver(&webhook, delivery, "", false)
	}

	return nil
}

// dueWebhookDeliveries returns the deliveries the retry worker should attempt now.
func dueWebhookDeliveries(db *gorm.DB, now time.Time) ([]models.WebhookDelivery, error) {
	var deliveries []models.WebhookDelivery
	err := db.Where(
		"dead_lettered_at IS NULL AND ("+
			"(status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR "+
			"(status = ? AND next_retry_at <= ? AND retry_count < max_retries))",
		"pending", now, "failed", now,
	).Find(&deliveries).Error
	return deliveries, err
}

// ListDeadLetterDeliveries returns the permanently failed deliveries for
// webhooks owned by userID, most recently dead-lettered first.
func ListDeadLetterDeliveries(db *gorm.DB, userID uint) ([]models.WebhookDelivery, error) {
	var deliveries []models.WebhookDelivery
	err := db.Model(&models.WebhookDelivery{}).
		Select("webhook_deliveries.*").
		Joins("JOIN webhooks ON webhooks.id = webhook_deliveries.webhook_id AND webhooks.deleted_at IS NULL").
		Where("webhooks.user_id = ? AND webhook_deliveries.dead_lettered_at IS NOT NULL", userID).
		Order("webhook_deliveries.dead_lettered_at DESC").
		Find(&deliveries).Error
	return deliveries, err
}
