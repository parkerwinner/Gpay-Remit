package middleware

import (
	"github.com/gin-gonic/gin"
)

// TLSMiddleware redirects HTTP requests to HTTPS and sets HSTS headers
func TLSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Set HSTS header for security
		c.Writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Check if request is HTTP and redirect to HTTPS
		proto := c.GetHeader("X-Forwarded-Proto")
		if proto == "http" || (c.Request.TLS == nil && proto == "") {
			target := "https://" + c.Request.Host + c.Request.URL.Path
			if len(c.Request.URL.RawQuery) > 0 {
				target += "?" + c.Request.URL.RawQuery
			}
			c.Redirect(301, target)
			c.Abort()
			return
		}

		c.Next()
	}
}
