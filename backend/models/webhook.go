package models

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Gpay-Remit/gpay-remit/encryption"
	"gorm.io/gorm"
)

var webhookEventPattern = regexp.MustCompile(`^(\*|[a-z][a-z0-9_]*\.(\*|[a-z][a-z0-9_]*))$`)

// WebhookEvents stores subscribed webhook event patterns as a JSON array in the database.
type WebhookEvents []string

// Value implements driver.Valuer to serialize WebhookEvents as a JSON array.
func (e WebhookEvents) Value() (driver.Value, error) {
	if e == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(e))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner to deserialize JSON array (or legacy comma-separated) events.
func (e *WebhookEvents) Scan(value interface{}) error {
	if value == nil {
		*e = WebhookEvents{}
		return nil
	}
	var raw string
	switch v := value.(type) {
	case []byte:
		raw = string(v)
	case string:
		raw = v
	default:
		return fmt.Errorf("unsupported type %T for WebhookEvents", value)
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		*e = WebhookEvents{}
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var list []string
		if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
			return err
		}
		*e = WebhookEvents(list)
		return nil
	}
	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	*e = WebhookEvents(out)
	return nil
}

// ValidateWebhookEventName checks that an event name or wildcard pattern is well-formed
// (e.g., "*", "payment.*", "payment.created").
func ValidateWebhookEventName(event string) error {
	e := strings.ToLower(strings.TrimSpace(event))
	if e == "" {
		return errors.New("webhook event name cannot be empty")
	}
	if !webhookEventPattern.MatchString(e) {
		return fmt.Errorf("invalid webhook event name %q: must be '*', '<resource>.*', or '<resource>.<action>'", event)
	}
	return nil
}

// NormalizeAndValidateWebhookEvents validates, normalizes, and deduplicates event patterns.
func NormalizeAndValidateWebhookEvents(events []string) (WebhookEvents, error) {
	if len(events) == 0 {
		return nil, errors.New("at least one webhook event is required")
	}
	seen := make(map[string]struct{}, len(events))
	out := make(WebhookEvents, 0, len(events))
	for _, raw := range events {
		e := strings.ToLower(strings.TrimSpace(raw))
		if err := ValidateWebhookEventName(e); err != nil {
			return nil, err
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	return out, nil
}

// MatchesWebhookEvent reports whether a subscribed pattern matches a concrete event.
// Supports exact matches, global wildcard ("*"), and namespace wildcards ("payment.*").
func MatchesWebhookEvent(pattern, event string) bool {
	p := strings.ToLower(strings.TrimSpace(pattern))
	ev := strings.ToLower(strings.TrimSpace(event))
	if p == "" || ev == "" {
		return false
	}
	if p == "*" || p == ev {
		return true
	}
	if strings.HasSuffix(p, ".*") {
		prefix := strings.TrimSuffix(p, "*") // keeps trailing "."
		return strings.HasPrefix(ev, prefix)
	}
	return false
}

const webhookSecretCipherPrefix = "enc:v1:"

type Webhook struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	UserID      uint           `gorm:"index;not null" json:"user_id"`
	URL         string         `gorm:"size:500;not null" json:"url"`
	Secret      string         `gorm:"size:512;not null" json:"-"`         // Encrypted HMAC secret for signature verification
	Events      WebhookEvents  `gorm:"type:jsonb;not null" json:"events"`  // JSON array of subscribed event patterns
	IsActive    bool           `gorm:"index;default:true" json:"is_active"`
	Description string         `gorm:"size:255" json:"description"`
}

// SubscribedTo reports whether this webhook matches the given event name.
func (w *Webhook) SubscribedTo(event string) bool {
	for _, pattern := range w.Events {
		if MatchesWebhookEvent(pattern, event) {
			return true
		}
	}
	return false
}

type WebhookDelivery struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	WebhookID    uint           `gorm:"index;not null" json:"webhook_id"`
	Event        string         `gorm:"size:100;not null" json:"event"`
	Payload      string         `gorm:"type:text;not null" json:"payload"`
	Status       string         `gorm:"index;size:20;default:'pending'" json:"status"` // pending, success, failed (failed + next_retry_at = retry scheduled; failed + dead_lettered_at = dead-lettered)
	ResponseCode int            `json:"response_code"`
	ResponseBody string         `gorm:"type:text" json:"response_body"`
	ErrorMessage string         `gorm:"type:text" json:"error_message"`
	AttemptCount int            `gorm:"default:0" json:"attempt_count"`
	NextRetryAt  *time.Time     `json:"next_retry_at"`
	CompletedAt  *time.Time     `json:"completed_at"`

	// MaxRetries caps scheduled (worker-driven) retries after the initial
	// attempt. Zero means DefaultWebhookMaxRetries.
	MaxRetries int `gorm:"default:6" json:"max_retries"`
	// RetryCount is how many scheduled retries have been scheduled so far.
	// It selects the backoff interval from WebhookRetryBackoff.
	RetryCount int `gorm:"default:0" json:"retry_count"`
	// DeadLetteredAt is set when the delivery has permanently failed (client
	// error, or retries exhausted). Such rows are never retried automatically
	// and form the dead-letter queue.
	DeadLetteredAt *time.Time `gorm:"index" json:"dead_lettered_at"`
}

// DefaultWebhookMaxRetries is the number of scheduled retries after the first
// attempt. It matches the length of WebhookRetryBackoff, so every backoff step
// is used exactly once before the delivery is dead-lettered.
const DefaultWebhookMaxRetries = 6

// WebhookRetryBackoff is the delay before each scheduled retry, in order:
// 1m, 5m, 15m, 1h, 6h, 24h. After the last step the delivery is given up.
var WebhookRetryBackoff = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	1 * time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

func encryptWebhookSecret(secret string) (string, error) {
	if secret == "" || strings.HasPrefix(secret, webhookSecretCipherPrefix) {
		return secret, nil
	}

	enc := encryption.GetDefaultEncryptor()
	if enc == nil {
		return "", errors.New("encryption: no default encryptor configured")
	}

	ciphertext, err := enc.EncryptString(context.Background(), secret)
	if err != nil {
		return "", fmt.Errorf("encrypting webhook secret: %w", err)
	}

	return webhookSecretCipherPrefix + ciphertext, nil
}

func decryptWebhookSecret(secret string) (string, error) {
	if !strings.HasPrefix(secret, webhookSecretCipherPrefix) {
		return secret, nil
	}

	enc := encryption.GetDefaultEncryptor()
	if enc == nil {
		return "", errors.New("encryption: no default encryptor configured")
	}

	plaintext, err := enc.DecryptString(context.Background(), strings.TrimPrefix(secret, webhookSecretCipherPrefix))
	if err != nil {
		return "", fmt.Errorf("decrypting webhook secret: %w", err)
	}

	return plaintext, nil
}

// BeforeSave encrypts a plaintext signing secret before it reaches the database.
func (w *Webhook) BeforeSave(tx *gorm.DB) error {
	encrypted, err := encryptWebhookSecret(w.Secret)
	if err != nil {
		return err
	}
	w.Secret = encrypted
	return nil
}

// AfterFind transparently decrypts the signing secret after a database read.
func (w *Webhook) AfterFind(tx *gorm.DB) error {
	decrypted, err := decryptWebhookSecret(w.Secret)
	if err != nil {
		return err
	}
	w.Secret = decrypted
	return nil
}

// EncryptLegacyWebhookSecrets backfills plaintext secrets written before the
// encryption hooks were introduced. It is safe to run on every startup.
func EncryptLegacyWebhookSecrets(db *gorm.DB) error {
	var webhooks []Webhook
	if err := db.Where("secret NOT LIKE ?", webhookSecretCipherPrefix+"%").Find(&webhooks).Error; err != nil {
		return fmt.Errorf("loading legacy webhook secrets: %w", err)
	}

	for i := range webhooks {
		encrypted, err := encryptWebhookSecret(webhooks[i].Secret)
		if err != nil {
			return err
		}
		if err := db.Model(&Webhook{}).Where("id = ?", webhooks[i].ID).
			UpdateColumn("secret", encrypted).Error; err != nil {
			return fmt.Errorf("encrypting webhook secret for webhook %d: %w", webhooks[i].ID, err)
		}
	}

	return nil
}

// TableName overrides the table name
func (Webhook) TableName() string {
	return "webhooks"
}

// TableName overrides the table name
func (WebhookDelivery) TableName() string {
	return "webhook_deliveries"
}
