package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	apperrors "github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// Claims represents the JWT claims
type Claims struct {
	UserID uint   `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken creates a new JWT token for a user
func GenerateToken(userID uint, role string, secret string, expiry time.Duration) (string, error) {
	expirationTime := time.Now().Add(expiry)
	jtiBytes := make([]byte, 16)
	rand.Read(jtiBytes)
	jti := hex.EncodeToString(jtiBytes)

	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        jti,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// TokenIsStale reports whether a token was issued at or before the user's
// password-change boundary. NumericDate has one-second resolution, so tokens
// minted in the same second as a password change are intentionally rejected.
func TokenIsStale(claims *Claims, passwordChangedAt *time.Time) bool {
	if passwordChangedAt == nil {
		return false
	}
	if claims.IssuedAt == nil {
		return true
	}

	issuedAt := claims.IssuedAt.Time.Truncate(time.Second)
	changedAt := passwordChangedAt.Truncate(time.Second)
	return !issuedAt.After(changedAt)
}

// JwtAuthMiddleware validates the JWT token and sets user info in the context
func JwtAuthMiddleware(db *gorm.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			AbortWithError(c, http.StatusUnauthorized, apperrors.CodeUnauthorized, "Authorization header is required", nil)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			AbortWithError(c, http.StatusUnauthorized, apperrors.CodeUnauthorized, "Invalid authorization header format", nil)
			return
		}

		tokenString := parts[1]

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(cfg.JWTSecret), nil
		})

		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				AbortWithError(c, http.StatusUnauthorized, apperrors.CodeUnauthorized, "Token has expired", gin.H{"reason": "ExpiredToken"})
			} else {
				AbortWithError(c, http.StatusUnauthorized, apperrors.CodeUnauthorized, "Invalid token", gin.H{"reason": "InvalidToken"})
			}
			return
		}

		if !token.Valid {
			AbortWithError(c, http.StatusUnauthorized, apperrors.CodeUnauthorized, "Invalid token", gin.H{"reason": "InvalidToken"})
			return
		}

		// Check if token is revoked
		if claims.ID != "" {
			var isRevoked bool
			found, err := utils.GetCached("revoked:"+claims.ID, &isRevoked)
			if err == nil && found && isRevoked {
				AbortWithError(c, http.StatusUnauthorized, apperrors.CodeUnauthorized, "Token has been revoked", gin.H{"reason": "RevokedToken"})
				return
			}
		}

		var tokenState struct {
			PasswordChangedAt *time.Time
		}
		if err := db.Model(&models.User{}).Select("password_changed_at").
			Where("id = ?", claims.UserID).Take(&tokenState).Error; err != nil {
			AbortWithError(c, http.StatusUnauthorized, apperrors.CodeUnauthorized, "Invalid token", gin.H{"reason": "InvalidToken"})
			return
		}
		if TokenIsStale(claims, tokenState.PasswordChangedAt) {
			AbortWithError(c, http.StatusUnauthorized, apperrors.CodeUnauthorized, "Token was issued before the last password change", gin.H{"reason": "StaleToken"})
			return
		}

		// Set user information in context
		c.Set("userID", claims.UserID)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// RequireRole checks if the user has specific roles
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists {
			AbortWithError(c, http.StatusUnauthorized, apperrors.CodeUnauthorized, "User role not found in context", nil)
			return
		}

		roleStr, ok := userRole.(string)
		if !ok {
			AbortWithError(c, http.StatusInternalServerError, apperrors.CodeInternal, "Invalid role type in context", nil)
			return
		}

		hasRole := false
		for _, role := range roles {
			if roleStr == role {
				hasRole = true
				break
			}
		}

		if !hasRole {
			AbortWithError(c, http.StatusForbidden, apperrors.CodeForbidden, "Forbidden: insufficient permissions", nil)
			return
		}

		c.Next()
	}
}
