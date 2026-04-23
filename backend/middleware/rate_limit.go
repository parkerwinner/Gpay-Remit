package middleware

import (
	"github.com/gin-gonic/gin"
)

func RateLimitMiddleWare() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Mock rate limiter
		c.Header("X-RateLimit-Limit", "100")
		c.Header("X-RateLimit-Remaining", "99")
		c.Next()
	}
}
