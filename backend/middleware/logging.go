package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/yourusername/gpay-remit/logger"
)

// RequestLogger returns a Gin middleware that logs every HTTP request with
// method, path, status code, duration, and client IP as structured fields.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		logger.Log.WithFields(logrus.Fields{
			"method":   c.Request.Method,
			"path":     path,
			"status":   c.Writer.Status(),
			"duration": time.Since(start).String(),
			"ip":       c.ClientIP(),
		}).Info("HTTP request")
	}
}
