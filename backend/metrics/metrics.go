package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Technical metrics: HTTP request counts, latency histogram, errors
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests processed.",
		},
		[]string{"method", "handler", "status"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gpay_remit",
			Name:      "http_request_duration_seconds",
			Help:      "Histogram of HTTP request latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "handler"},
	)

	SystemErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "system_errors_total",
			Help:      "Total number of system and application errors.",
		},
		[]string{"type", "component"},
	)

	// Business metrics: Payments, amounts, error tracking
	PaymentsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "payments_total",
			Help:      "Total number of payments processed.",
		},
		[]string{"status", "currency"},
	)

	PaymentAmountTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "payment_amount_total",
			Help:      "Cumulative volume of payment amounts processed.",
		},
		[]string{"currency"},
	)

	PaymentErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "payment_errors_total",
			Help:      "Total number of failed payment attempts.",
		},
		[]string{"reason", "component"},
	)
)

var (
	// ExchangeRateLookupsTotal counts exchange rate lookups by where the rate
	// was served from: cache, oracle or stale. A rising stale share means the
	// oracle is unhealthy; a low cache share means the TTL is too short.
	ExchangeRateLookupsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "exchange_rate_lookups_total",
			Help:      "Exchange rate lookups by served source (cache, oracle, stale).",
		},
		[]string{"source"},
	)

	// WebhookRetryAttemptsTotal counts every webhook delivery attempt that is a
	// retry (not the first attempt). kind is "inline" for short in-line retries
	// of the initial delivery or "scheduled" for backoff-worker retries; outcome
	// is "success" or "failure".
	WebhookRetryAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "webhook_retry_attempts_total",
			Help:      "Webhook delivery retry attempts by kind and outcome.",
		},
		[]string{"kind", "outcome"},
	)

	// WebhookDeadLettersTotal counts deliveries moved to the dead-letter queue.
	// reason is "client_error" (4xx, never retried) or "retries_exhausted".
	WebhookDeadLettersTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "webhook_dead_letters_total",
			Help:      "Webhook deliveries moved to the dead-letter queue by reason.",
		},
		[]string{"reason"},
	)

	CacheOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "cache_operations_total",
			Help:      "Cache operations by operation and outcome (hit, miss, success, error, unavailable).",
		},
		[]string{"operation", "result"},
	)

	RedisEvictedKeysTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "redis_evicted_keys_total",
			Help:      "Redis key evictions observed by this process; use rate() to alert on eviction pressure.",
		},
	)

	WebhookDeliveriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gpay_remit",
			Name:      "webhook_deliveries_total",
			Help:      "Webhook delivery attempts by outcome.",
		},
		[]string{"outcome"},
	)

	DBConnectionsInUse = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gpay_remit",
			Name:      "db_connections_in_use",
			Help:      "Current number of in-use database connections.",
		},
	)

	DBConnectionsMax = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gpay_remit",
			Name:      "db_connections_max",
			Help:      "Configured maximum number of database connections.",
		},
	)

	RedisUp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gpay_remit",
			Name:      "redis_up",
			Help:      "Whether Redis is reachable (1 for reachable, 0 otherwise).",
		},
	)
)

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		SystemErrorsTotal,
		PaymentsTotal,
		PaymentAmountTotal,
		PaymentErrorsTotal,
		ExchangeRateLookupsTotal,
		WebhookRetryAttemptsTotal,
		WebhookDeadLettersTotal,
		CacheOperationsTotal,
		RedisEvictedKeysTotal,
		WebhookDeliveriesTotal,
		DBConnectionsInUse,
		DBConnectionsMax,
		RedisUp,
	)
}

// Handler returns standard Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

// RecordHTTPRequest updates HTTP request counters and duration histograms.
func RecordHTTPRequest(method, handler, status string, durationSeconds float64) {
	HTTPRequestsTotal.WithLabelValues(method, handler, status).Inc()
	HTTPRequestDuration.WithLabelValues(method, handler).Observe(durationSeconds)
}

// RecordPayment records business metrics for payment operations.
func RecordPayment(status, currency string, amount float64) {
	PaymentsTotal.WithLabelValues(status, currency).Inc()
	if amount > 0 {
		PaymentAmountTotal.WithLabelValues(currency).Add(amount)
	}
}

// RecordPaymentError increments the payment error counter.
func RecordPaymentError(reason, component string) {
	PaymentErrorsTotal.WithLabelValues(reason, component).Inc()
}

// RecordSystemError increments system error counters.
func RecordSystemError(errType, component string) {
	SystemErrorsTotal.WithLabelValues(errType, component).Inc()
}
