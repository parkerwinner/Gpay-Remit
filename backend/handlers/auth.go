package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/middleware"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB  *gorm.DB
	Cfg *config.Config
}

func NewAuthHandler(db *gorm.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		DB:  db,
		Cfg: cfg,
	}
}

// RefreshToken request body
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh handles token refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Validate refresh token using the refresh secret
	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(req.RefreshToken, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.Cfg.JWTRefreshSecret), nil
	})

	if err != nil || !token.Valid {
		c.Error(errors.NewUnauthorizedError("Invalid or expired refresh token"))
		return
	}

	// Fetch user from DB to ensure they still exist and are active
	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil {
		c.Error(errors.NewUnauthorizedError("User not found"))
		return
	}

	if !user.IsActive {
		c.Error(errors.NewForbiddenError("User account is inactive"))
		return
	}

	// Issue new access and refresh tokens
	accessToken, err := middleware.GenerateToken(user.ID, user.Role, h.Cfg.JWTSecret, 15*time.Minute)
	if err != nil {
		c.Error(errors.NewInternalError("Failed to generate access token", err))
		return
	}

	refreshToken, err := middleware.GenerateToken(user.ID, user.Role, h.Cfg.JWTRefreshSecret, 7*24*time.Hour)
	if err != nil {
		c.Error(errors.NewInternalError("Failed to generate refresh token", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}
