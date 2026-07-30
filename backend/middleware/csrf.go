package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CSRFProtection implements the double-submit cookie pattern
func CSRFProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method

		// Safe methods: ensure CSRF token exists in cookie
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			_, err := c.Cookie("csrf_token")
			if err != nil {
				// Generate new token
				b := make([]byte, 32)
				rand.Read(b)
				token := hex.EncodeToString(b)

				// HttpOnly=false so JS can read it for the header
				c.SetCookie("csrf_token", token, 3600*24, "/", "", false, false)
			}
			c.Next()
			return
		}

		// Unsafe methods: validate CSRF token
		cookieToken, err := c.Cookie("csrf_token")
		if err != nil || cookieToken == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Missing CSRF cookie"})
			c.Abort()
			return
		}

		headerToken := c.GetHeader("X-CSRF-Token")
		if headerToken == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Missing CSRF header"})
			c.Abort()
			return
		}

		if cookieToken != headerToken {
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid CSRF token"})
			c.Abort()
			return
		}

		c.Next()
	}
}
