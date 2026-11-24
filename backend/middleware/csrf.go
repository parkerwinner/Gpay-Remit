package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"os"

	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/gin-gonic/gin"
)

// CSRFProtection implements the double-submit cookie pattern with timing-safe checks
func CSRFProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		isProduction := os.Getenv("APP_ENV") == "production"

		// Safe methods: ensure CSRF token exists in cookie
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			_, err := c.Cookie("csrf_token")
			if err != nil {
				// Generate new token
				b := make([]byte, 32)
				_, _ = rand.Read(b)
				token := hex.EncodeToString(b)

				// HttpOnly=false so JS can read it for the header; SameSite=Lax and Secure in production
				c.SetSameSite(http.SameSiteLaxMode)
				c.SetCookie("csrf_token", token, 3600*24, "/", "", isProduction, false)
			}
			c.Next()
			return
		}

		// Unsafe methods: validate CSRF token
		cookieToken, err := c.Cookie("csrf_token")
		if err != nil || cookieToken == "" {
			AbortWithError(c, http.StatusForbidden, errors.CodeForbidden, "Missing CSRF cookie", nil)
			return
		}

		headerToken := c.GetHeader("X-CSRF-Token")
		if headerToken == "" {
			AbortWithError(c, http.StatusForbidden, errors.CodeForbidden, "Missing CSRF header", nil)
			return
		}

		// Timing-safe comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
			AbortWithError(c, http.StatusForbidden, errors.CodeForbidden, "Invalid CSRF token", nil)
			return
		}

		c.Next()
	}
}
