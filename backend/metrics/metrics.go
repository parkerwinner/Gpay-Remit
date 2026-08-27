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

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		SystemErrorsTotal,
		PaymentsTotal,
		PaymentAmountTotal,
		PaymentErrorsTotal,
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
