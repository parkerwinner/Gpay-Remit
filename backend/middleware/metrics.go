package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/metrics"
)

// PrometheusMetrics returns a Gin middleware that records request latency and status metrics.
func PrometheusMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", c.Writer.Status())
		handler := c.FullPath()
		if handler == "" {
			handler = c.Request.URL.Path
		}

		metrics.RecordHTTPRequest(c.Request.Method, handler, status, duration)

		if c.Writer.Status() >= 500 {
			metrics.RecordSystemError("http_5xx_error", handler)
		}
	}
}
