package services

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/Gpay-Remit/gpay-remit/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWebhookTestDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&models.Webhook{}, &models.WebhookDelivery{})
	return db
}

// Test error classification (#197)
func TestClassifyError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		statusCode     int
		expectedType   ErrorClassification
	}{
		{
			name:         "Timeout Error",
			err:          &net.OpError{Op: "dial", Net: "tcp", Err: &timeoutError{}},
			statusCode:   0,
			expectedType: ErrorTimeout,
		},
		{
			name:         "DNS Error",
			err:          &net.DNSError{Err: "no such host"},
			statusCode:   0,
			expectedType: ErrorDNS,
		},
		{
			name:         "Connection Refused",
			err:          &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
			statusCode:   0,
			expectedType: ErrorConnectionRefused,
		},
		{
			name:         "HTTP 4xx",
			err:          nil,
			statusCode:   400,
			expectedType: ErrorHTTP4xx,
		},
		{
			name:         "HTTP 5xx",
			err:          nil,
			statusCode:   500,
			expectedType: ErrorHTTP5xx,
		},
		{
			name:         "Other Error",
			err:          errors.New("unknown error"),
			statusCode:   0,
			expectedType: ErrorOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err, tt.statusCode)
			assert.Equal(t, tt.expectedType, result)
		})
	}
}

// Test retry policies (#197)
func TestGetRetryPolicy(t *testing.T) {
	tests := []struct {
		name           string
		errorType      ErrorClassification
		expectRetries  int
		expectDelay    time.Duration
		shouldRetry    bool
	}{
		{
			name:          "Timeout - should retry with exponential backoff",
			errorType:     ErrorTimeout,
			expectRetries: 3,
			expectDelay:   time.Second,
			shouldRetry:   true,
		},
		{
			name:          "DNS - should retry with fixed delay",
			errorType:     ErrorDNS,
			expectRetries: 2,
			expectDelay:   5 * time.Second,
			shouldRetry:   true,
		},
		{
			name:          "Connection Refused - should not retry immediately",
			errorType:     ErrorConnectionRefused,
			expectRetries: 0,
			expectDelay:   30 * time.Second,
			shouldRetry:   false,
		},
		{
			name:          "HTTP 4xx - should not retry",
			errorType:     ErrorHTTP4xx,
			expectRetries: 0,
			expectDelay:   0,
			shouldRetry:   false,
		},
		{
			name:          "HTTP 5xx - should retry",
			errorType:     ErrorHTTP5xx,
			expectRetries: 3,
			expectDelay:   time.Second,
			shouldRetry:   true,
		},
		{
			name:          "Other - should retry once",
			errorType:     ErrorOther,
			expectRetries: 1,
			expectDelay:   2 * time.Second,
			shouldRetry:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxRetries, baseDelay, shouldRetry := getRetryPolicy(tt.errorType)
			assert.Equal(t, tt.expectRetries, maxRetries)
			assert.Equal(t, tt.expectDelay, baseDelay)
			assert.Equal(t, tt.shouldRetry, shouldRetry)
		})
	}
}

// Test webhook delivery with different error types (#197)
func TestWebhookDeliveryErrorHandling(t *testing.T) {
	db := setupWebhookTestDB()
	
	t.Run("Success on Second Attempt", func(t *testing.T) {
		// Create a server that fails first then succeeds
		attempt := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempt++
			if attempt == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}))
		defer server.Close()

		service := NewWebhookDeliveryService(db)
		
		webhook := &models.Webhook{
			ID:     1,
			URL:    server.URL,
			Secret: "test-secret",
		}
		
		delivery := &models.WebhookDelivery{
			ID:        1,
			WebhookID: 1,
			Event:     "test.event",
			Payload:   `{"test": "data"}`,
			Status:    "pending",
		}
		db.Create(delivery)

		// This would normally run in a goroutine, but we'll call it directly for testing
		service.DeliverWebhook(webhook, delivery, "")

		// Check that delivery was marked as successful
		db.First(delivery)
		assert.Equal(t, "success", delivery.Status)
		assert.Equal(t, 2, delivery.AttemptCount)
		assert.NotNil(t, delivery.CompletedAt)
	})

	t.Run("Permanent Failure After Max Retries", func(t *testing.T) {
		// Create a server that always returns 4xx (should not retry)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("bad request"))
		}))
		defer server.Close()

		service := NewWebhookDeliveryService(db)
		
		webhook := &models.Webhook{
			ID:     2,
			URL:    server.URL,
			Secret: "test-secret",
		}
		
		delivery := &models.WebhookDelivery{
			ID:        2,
			WebhookID: 2,
			Event:     "test.event",
			Payload:   `{"test": "data"}`,
			Status:    "pending",
		}
		db.Create(delivery)

		service.DeliverWebhook(webhook, delivery, "")

		// Check that delivery was marked as permanently failed
		db.First(delivery)
		assert.Equal(t, "failed", delivery.Status)
		assert.Equal(t, 1, delivery.AttemptCount) // Should not retry 4xx errors
		assert.NotNil(t, delivery.CompletedAt)
	})
}

// Mock timeout error for testing
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

// Test connection refused detection
func TestIsConnectionRefused(t *testing.T) {
	t.Run("Valid Connection Refused", func(t *testing.T) {
		opErr := &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: syscall.ECONNREFUSED,
		}
		assert.True(t, isConnectionRefused(opErr))
	})

	t.Run("Different Error", func(t *testing.T) {
		opErr := &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: errors.New("other error"),
		}
		assert.False(t, isConnectionRefused(opErr))
	})

	t.Run("Nil Error", func(t *testing.T) {
		opErr := &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: nil,
		}
		assert.False(t, isConnectionRefused(opErr))
	})
}

// Tests for Webhook HMAC Signature and Tamper Detection (#424)
func TestWebhookDeliverySignatureAndTamperVerification(t *testing.T) {
	secret := "super-secure-webhook-secret-key"
	validPayload := `{"event":"payment.completed","timestamp":"2026-01-15T10:30:00Z","data":{"amount":"100.00","currency":"USDC"}}`

	service := NewWebhookDeliveryService(nil)
	validSig := service.generateSignature(secret, validPayload)
	currentTimestamp := strconv.FormatInt(time.Now().Unix(), 10)

	t.Run("Task 1: Delivers a webhook with a valid signature", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("X-Webhook-Signature", validSig)
		headers.Set("X-Webhook-Timestamp", currentTimestamp)

		err := VerifyWebhookDelivery(secret, headers, []byte(validPayload), 5*time.Minute)
		assert.NoError(t, err, "Webhook with valid signature and fresh timestamp should pass verification")
	})

	t.Run("Task 2: Test with a tampered body (signature should not match)", func(t *testing.T) {
		tamperedPayload := `{"event":"payment.completed","timestamp":"2026-01-15T10:30:00Z","data":{"amount":"999999.00","currency":"USDC"}}`

		headers := http.Header{}
		headers.Set("X-Webhook-Signature", validSig) // Using signature generated for validPayload
		headers.Set("X-Webhook-Timestamp", currentTimestamp)

		err := VerifyWebhookDelivery(secret, headers, []byte(tamperedPayload), 5*time.Minute)
		assert.Error(t, err, "Tampered payload should fail HMAC verification")
		assert.Contains(t, err.Error(), "invalid webhook signature")
	})

	t.Run("Task 3: Test with a missing signature header", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("X-Webhook-Timestamp", currentTimestamp)
		// No X-Webhook-Signature header set

		err := VerifyWebhookDelivery(secret, headers, []byte(validPayload), 5*time.Minute)
		assert.Error(t, err, "Missing signature header should fail verification")
		assert.Contains(t, err.Error(), "missing X-Webhook-Signature header")
	})

	t.Run("Task 4: Test with an expired timestamp", func(t *testing.T) {
		// Timestamp from 1 hour ago
		expiredTimestamp := strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10)

		headers := http.Header{}
		headers.Set("X-Webhook-Signature", validSig)
		headers.Set("X-Webhook-Timestamp", expiredTimestamp)

		// Verification with 5-minute maximum tolerance
		err := VerifyWebhookDelivery(secret, headers, []byte(validPayload), 5*time.Minute)
		assert.Error(t, err, "Expired timestamp should fail verification")
		assert.Contains(t, err.Error(), "webhook timestamp expired")
	})

	t.Run("End-to-End Delivery and Consumer Verification", func(t *testing.T) {
		receivedHeaders := make(chan http.Header, 1)
		receivedBody := make(chan string, 1)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders <- r.Header.Clone()
			b, _ := io.ReadAll(r.Body)
			receivedBody <- string(b)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		webhook := &models.Webhook{
			ID:     10,
			URL:    server.URL,
			Secret: secret,
		}

		success, code, _, errMsg, err := service.sendWebhookRequest(webhook, validPayload, "req-123")
		assert.True(t, success)
		assert.Equal(t, http.StatusOK, code)
		assert.NoError(t, err)
		assert.Empty(t, errMsg)

		headers := <-receivedHeaders
		body := <-receivedBody

		// Consumer verifies received payload
		verifyErr := VerifyWebhookDelivery(secret, headers, []byte(body), 5*time.Minute)
		assert.NoError(t, verifyErr, "Consumer should successfully verify delivered webhook")

		// Consumer rejects tampered payload
		tamperedErr := VerifyWebhookDelivery(secret, headers, []byte(body+`{"extra":true}`), 5*time.Minute)
		assert.Error(t, tamperedErr, "Consumer must reject tampered body")
	})
}