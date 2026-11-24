package models

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Gpay-Remit/gpay-remit/encryption"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWebhookDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Webhook{}))
	return db
}

func useWebhookTestEncryptor(t *testing.T) {
	t.Helper()
	enc, err := encryption.NewAESGCMEncryptor([]byte("webhook-secret-test-key-1234567890"))
	require.NoError(t, err)
	original := encryption.GetDefaultEncryptor()
	encryption.SetDefaultEncryptor(enc)
	t.Cleanup(func() { encryption.SetDefaultEncryptor(original) })
}

func rawWebhookSecret(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var rows []struct {
		Secret string
	}
	require.NoError(t, db.Raw("SELECT secret FROM webhooks WHERE id = ?", id).Scan(&rows).Error)
	require.Len(t, rows, 1)
	return rows[0].Secret
}

func TestWebhookSecretEncryptedAtRestAndDecryptedOnRead(t *testing.T) {
	useWebhookTestEncryptor(t)
	db := setupWebhookDB(t)
	plaintext := "whsec_test_secret_value"

	webhook := Webhook{
		UserID: 1,
		URL:    "https://example.com/webhook",
		Secret: plaintext,
		Events: WebhookEvents{"payment.completed"},
	}
	require.NoError(t, db.Create(&webhook).Error)

	stored := rawWebhookSecret(t, db, webhook.ID)
	assert.NotEqual(t, plaintext, stored)
	assert.True(t, strings.HasPrefix(stored, webhookSecretCipherPrefix))

	var loaded Webhook
	require.NoError(t, db.First(&loaded, webhook.ID).Error)
	assert.Equal(t, plaintext, loaded.Secret)

	loaded.Description = "updated"
	require.NoError(t, db.Save(&loaded).Error)
	updatedStored := rawWebhookSecret(t, db, webhook.ID)
	assert.NotEqual(t, stored, updatedStored)
	assert.True(t, strings.HasPrefix(updatedStored, webhookSecretCipherPrefix))

	serialized, err := json.Marshal(loaded)
	require.NoError(t, err)
	assert.NotContains(t, string(serialized), plaintext)
	assert.NotContains(t, string(serialized), "secret")
}

func TestEncryptLegacyWebhookSecrets(t *testing.T) {
	useWebhookTestEncryptor(t)
	db := setupWebhookDB(t)
	plaintext := "whsec_legacy_secret_value"

	require.NoError(t, db.Exec(
		"INSERT INTO webhooks (user_id, url, secret, events, is_active) VALUES (?, ?, ?, ?, ?)",
		1, "https://legacy.example.com/hook", plaintext, "payment.completed", true,
	).Error)

	var webhook Webhook
	require.NoError(t, db.Where("secret = ?", plaintext).First(&webhook).Error)
	assert.Equal(t, plaintext, rawWebhookSecret(t, db, webhook.ID))

	require.NoError(t, EncryptLegacyWebhookSecrets(db))
	encrypted := rawWebhookSecret(t, db, webhook.ID)
	assert.NotEqual(t, plaintext, encrypted)
	assert.True(t, strings.HasPrefix(encrypted, webhookSecretCipherPrefix))

	var loaded Webhook
	require.NoError(t, db.First(&loaded, webhook.ID).Error)
	assert.Equal(t, plaintext, loaded.Secret)

	require.NoError(t, EncryptLegacyWebhookSecrets(db))
	assert.Equal(t, encrypted, rawWebhookSecret(t, db, webhook.ID))
}

func TestWebhookEvents_ValidationAndWildcardMatching(t *testing.T) {
	// Valid patterns
	normalized, err := NormalizeAndValidateWebhookEvents([]string{" Payment.Created ", "payment.*", "payment.created"})
	require.NoError(t, err)
	assert.Equal(t, WebhookEvents{"payment.created", "payment.*"}, normalized)

	// Invalid patterns
	_, err = NormalizeAndValidateWebhookEvents([]string{})
	assert.Error(t, err)
	_, err = NormalizeAndValidateWebhookEvents([]string{"invalid-event"})
	assert.Error(t, err)
	_, err = NormalizeAndValidateWebhookEvents([]string{"payment.*.extra"})
	assert.Error(t, err)

	// Wildcard matching
	wh := Webhook{Events: WebhookEvents{"payment.*", "invoice.paid"}}
	assert.True(t, wh.SubscribedTo("payment.created"))
	assert.True(t, wh.SubscribedTo("payment.completed"))
	assert.True(t, wh.SubscribedTo("invoice.paid"))
	assert.False(t, wh.SubscribedTo("payment_request.created"))
	assert.False(t, wh.SubscribedTo("invoice.failed"))

	all := Webhook{Events: WebhookEvents{"*"}}
	assert.True(t, all.SubscribedTo("anything.happened"))
}

func TestWebhookEvents_JSONArrayPersistenceAndLegacyScan(t *testing.T) {
	useWebhookTestEncryptor(t)
	db := setupWebhookDB(t)

	wh := Webhook{
		UserID: 1,
		URL:    "https://example.com/hook",
		Secret: "whsec_json_array_secret",
		Events: WebhookEvents{"payment.*", "invoice.paid"},
	}
	require.NoError(t, db.Create(&wh).Error)

	var rawEvents string
	require.NoError(t, db.Raw("SELECT events FROM webhooks WHERE id = ?", wh.ID).Scan(&rawEvents).Error)
	assert.Equal(t, `["payment.*","invoice.paid"]`, rawEvents)

	var loaded Webhook
	require.NoError(t, db.First(&loaded, wh.ID).Error)
	assert.Equal(t, WebhookEvents{"payment.*", "invoice.paid"}, loaded.Events)

	// Legacy comma-separated string scan compatibility
	var legacy WebhookEvents
	require.NoError(t, legacy.Scan("payment.created, payment.failed"))
	assert.Equal(t, WebhookEvents{"payment.created", "payment.failed"}, legacy)
}
