package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/config"
)

func TestJwtAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		JWTSecret: "test-secret",
	}

	validToken, _ := GenerateToken(1, "user", cfg.JWTSecret, 1*time.Hour)
	expiredToken, _ := GenerateToken(1, "user", cfg.JWTSecret, -1*time.Hour)

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		expectedRole   string
		expectedCode   string
	}{
		{
			name:           "Valid Token",
			authHeader:     "Bearer " + validToken,
			expectedStatus: http.StatusOK,
			expectedRole:   "user",
		},
		{
			name:           "Missing Header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid Format",
			authHeader:     "Invalid " + validToken,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Expired Token",
			authHeader:     "Bearer " + expiredToken,
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "ExpiredToken",
		},
		{
			name:           "Invalid Token",
			authHeader:     "Bearer invalid.token.string",
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "InvalidToken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(JwtAuthMiddleware(cfg))
			router.GET("/test", func(c *gin.Context) {
				role, _ := c.Get("role")
				c.JSON(http.StatusOK, gin.H{"role": role})
			})

			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedCode != "" {
				assert.Contains(t, w.Body.String(), tt.expectedCode)
			}
		})
	}
}

func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		setupContext   func(c *gin.Context)
		requiredRoles  []string
		expectedStatus int
	}{
		{
			name: "Has Required Role",
			setupContext: func(c *gin.Context) {
				c.Set("role", "admin")
			},
			requiredRoles:  []string{"admin"},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Has One Of Required Roles",
			setupContext: func(c *gin.Context) {
				c.Set("role", "superadmin")
			},
			requiredRoles:  []string{"admin", "superadmin"},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Missing Required Role",
			setupContext: func(c *gin.Context) {
				c.Set("role", "user")
			},
			requiredRoles:  []string{"admin"},
			expectedStatus: http.StatusForbidden,
		},
		{
			name: "No Role In Context",
			setupContext: func(c *gin.Context) {
			},
			requiredRoles:  []string{"admin"},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				tt.setupContext(c)
				c.Next()
			})
			router.Use(RequireRole(tt.requiredRoles...))
			router.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
