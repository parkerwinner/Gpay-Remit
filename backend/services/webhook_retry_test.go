package services

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gpay-Remit/gpay-remit/metrics"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// counterValue reads a counter's current value. It avoids the testutil package,
// which would need go.mod changes.
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}

// failingEndpoint returns a server answering every request with status, and a
// counter of how many requests reached it.
func failingEndpoint(t *testing.T, status int) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte("nope"))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func seedRetryWebhook(t *testing.T, db *gorm.DB, url string, userID uint) *models.Webhook {
	t.Helper()
	wh := &models.Webhook{UserID: userID, URL: url, Secret: "s", Events: models.WebhookEvents{"payment.completed"}, IsActive: true}
	require.NoError(t, db.Create(wh).Error)
	return wh
}

func seedRetryDelivery(t *testing.T, db *gorm.DB, webhookID uint, mutate func(*models.WebhookDelivery)) *models.WebhookDelivery {
	t.Helper()
	d := &models.WebhookDelivery{
		WebhookID:  webhookID,
		Event:      "payment.completed",
		Payload:    `{"event":"payment.completed"}`,
		Status:     "pending",
		MaxRetries: models.DefaultWebhookMaxRetries,
	}
	if mutate != nil {
		mutate(d)
	}
	require.NoError(t, db.Create(d).Error)
	return d
}

func reloadDelivery(t *testing.T, db *gorm.DB, id uint) models.WebhookDelivery {
	t.Helper()
	var d models.WebhookDelivery
	require.NoError(t, db.First(&d, id).Error)
	return d
}

func TestBackoffLadderMatchesSpec(t *testing.T) {
	// The spec is: 1m, 5m, 15m, 1h, 6h, 24h, then give up.
	assert.Equal(t, []time.Duration{
		time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour,
	}, models.WebhookRetryBackoff)
	assert.Equal(t, len(models.WebhookRetryBackoff), models.DefaultWebhookMaxRetries)
}

// A worker-driven retry (allowInline=false) makes one attempt per step, so the
// test walks the whole schedule without sleeping. Each failure must schedule
// the next step at the documented delay, and the attempt after the last step
// must dead-letter the delivery.
func TestScheduledRetriesFollowBackoffThenDeadLetter(t *testing.T) {
	db := setupWebhookTestDB()
	srv, hits := failingEndpoint(t, http.StatusInternalServerError)
	wh := seedRetryWebhook(t, db, srv.URL, 1)
	d := seedRetryDelivery(t, db, wh.ID, nil)
	svc := NewWebhookDeliveryService(db)

	// Every call below takes the worker path (allowInline=false), so no in-line
	// sleeps happen and the test runs in milliseconds.
	for step, want := range models.WebhookRetryBackoff {
		cur := reloadDelivery(t, db, d.ID)
		before := time.Now()
		svc.deliver(wh, &cur, "", false)

		got := reloadDelivery(t, db, d.ID)
		require.Equal(t, "failed", got.Status, "step %d", step)
		require.Equal(t, step+1, got.RetryCount, "step %d", step)
		require.Nil(t, got.DeadLetteredAt, "step %d must not dead-letter yet", step)
		require.NotNil(t, got.NextRetryAt, "step %d", step)

		delay := got.NextRetryAt.Sub(before)
		assert.InDelta(t, float64(want), float64(delay), float64(5*time.Second), "step %d delay", step)
	}

	// The next worker attempt is the seventh and last one: give up.
	cur := reloadDelivery(t, db, d.ID)
	svc.deliver(wh, &cur, "", false)
	final := reloadDelivery(t, db, d.ID)
	assert.NotNil(t, final.DeadLetteredAt, "retries exhausted: must be dead-lettered")
	assert.Nil(t, final.NextRetryAt, "dead-lettered delivery is never rescheduled")
	assert.NotNil(t, final.CompletedAt)
	assert.Equal(t, models.DefaultWebhookMaxRetries, final.RetryCount)
	assert.Equal(t, models.DefaultWebhookMaxRetries+1, final.AttemptCount, "seven attempts: the first plus six scheduled retries")
	assert.Equal(t, int64(models.DefaultWebhookMaxRetries+1), hits.Load())
}

func TestScheduledRetryHonoursMaxRetries(t *testing.T) {
	db := setupWebhookTestDB()
	srv, _ := failingEndpoint(t, http.StatusBadGateway)
	wh := seedRetryWebhook(t, db, srv.URL, 1)
	d := seedRetryDelivery(t, db, wh.ID, func(d *models.WebhookDelivery) { d.MaxRetries = 2 })
	svc := NewWebhookDeliveryService(db)

	before := counterValue(t, metrics.WebhookDeadLettersTotal.WithLabelValues("retries_exhausted"))
	retryFailBefore := counterValue(t, metrics.WebhookRetryAttemptsTotal.WithLabelValues("scheduled", "failure"))

	for i := 0; i < 2; i++ {
		cur := reloadDelivery(t, db, d.ID)
		svc.deliver(wh, &cur, "", false)
		got := reloadDelivery(t, db, d.ID)
		require.Nil(t, got.DeadLetteredAt, "retry %d is within the cap", i+1)
	}

	cur := reloadDelivery(t, db, d.ID)
	svc.deliver(wh, &cur, "", false)
	final := reloadDelivery(t, db, d.ID)
	require.NotNil(t, final.DeadLetteredAt, "third failure exceeds max_retries=2")
	assert.Equal(t, 2, final.RetryCount)

	assert.Equal(t, before+1, counterValue(t, metrics.WebhookDeadLettersTotal.WithLabelValues("retries_exhausted")))
	// Three attempts in total: the first is not a retry, the next two are.
	assert.Equal(t, retryFailBefore+2, counterValue(t, metrics.WebhookRetryAttemptsTotal.WithLabelValues("scheduled", "failure")),
		"each failed retry attempt emits a metric")
}

func TestClientErrorIsDeadLetteredWithoutRetry(t *testing.T) {
	db := setupWebhookTestDB()
	srv, hits := failingEndpoint(t, http.StatusNotFound)
	wh := seedRetryWebhook(t, db, srv.URL, 1)
	d := seedRetryDelivery(t, db, wh.ID, nil)
	svc := NewWebhookDeliveryService(db)

	before := counterValue(t, metrics.WebhookDeadLettersTotal.WithLabelValues("client_error"))
	cur := reloadDelivery(t, db, d.ID)
	svc.DeliverWebhook(wh, &cur, "")

	final := reloadDelivery(t, db, d.ID)
	assert.Equal(t, "failed", final.Status)
	assert.NotNil(t, final.DeadLetteredAt)
	assert.Nil(t, final.NextRetryAt)
	assert.Equal(t, 0, final.RetryCount, "a 4xx never enters the backoff schedule")
	assert.Equal(t, int64(1), hits.Load(), "a 4xx is sent exactly once")
	assert.Equal(t, before+1, counterValue(t, metrics.WebhookDeadLettersTotal.WithLabelValues("client_error")))
}

func TestDueDeliveriesSelection(t *testing.T) {
	db := setupWebhookTestDB()
	wh := seedRetryWebhook(t, db, "http://unused.invalid", 1)
	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	deadAt := now.Add(-time.Hour)

	due := seedRetryDelivery(t, db, wh.ID, func(d *models.WebhookDelivery) { d.Status = "pending" })
	dueFailed := seedRetryDelivery(t, db, wh.ID, func(d *models.WebhookDelivery) {
		d.Status = "failed"
		d.NextRetryAt = &past
		d.RetryCount = 3
	})
	exhausted := seedRetryDelivery(t, db, wh.ID, func(d *models.WebhookDelivery) {
		d.Status = "failed"
		d.NextRetryAt = &past
		d.RetryCount = models.DefaultWebhookMaxRetries
	})
	deadLettered := seedRetryDelivery(t, db, wh.ID, func(d *models.WebhookDelivery) {
		d.Status = "failed"
		d.NextRetryAt = &past
		d.DeadLetteredAt = &deadAt
	})
	notYetDue := seedRetryDelivery(t, db, wh.ID, func(d *models.WebhookDelivery) {
		d.Status = "failed"
		d.NextRetryAt = &future
	})
	terminalNoSchedule := seedRetryDelivery(t, db, wh.ID, func(d *models.WebhookDelivery) {
		d.Status = "failed"
		d.NextRetryAt = nil
	})
	succeeded := seedRetryDelivery(t, db, wh.ID, func(d *models.WebhookDelivery) { d.Status = "success" })

	got, err := dueWebhookDeliveries(db, now)
	require.NoError(t, err)

	ids := make(map[uint]bool)
	for _, d := range got {
		ids[d.ID] = true
	}
	assert.True(t, ids[due.ID], "never-attempted pending delivery is due")
	assert.True(t, ids[dueFailed.ID], "failed delivery past its backoff with retries left is due")
	assert.False(t, ids[exhausted.ID], "exhausted delivery must be skipped")
	assert.False(t, ids[deadLettered.ID], "dead-lettered delivery must be skipped")
	assert.False(t, ids[notYetDue.ID], "delivery still in backoff is not due")
	assert.False(t, ids[terminalNoSchedule.ID], "failed delivery without a schedule is terminal")
	assert.False(t, ids[succeeded.ID])
	assert.Len(t, got, 2)
}

func TestListDeadLetterDeliveriesIsScopedToOwner(t *testing.T) {
	db := setupWebhookTestDB()
	mine := seedRetryWebhook(t, db, "https://mine.example", 1)
	theirs := seedRetryWebhook(t, db, "https://theirs.example", 2)
	now := time.Now()

	myDead := seedRetryDelivery(t, db, mine.ID, func(d *models.WebhookDelivery) {
		d.Status = "failed"
		d.DeadLetteredAt = &now
	})
	seedRetryDelivery(t, db, theirs.ID, func(d *models.WebhookDelivery) {
		d.Status = "failed"
		d.DeadLetteredAt = &now
	})
	seedRetryDelivery(t, db, mine.ID, func(d *models.WebhookDelivery) { d.Status = "pending" })

	got, err := ListDeadLetterDeliveries(db, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, myDead.ID, got[0].ID)
}
