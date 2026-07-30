package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/logger"
	"github.com/yourusername/gpay-remit/middleware"
	"github.com/yourusername/gpay-remit/models"
	"github.com/yourusername/gpay-remit/services"
	"github.com/yourusername/gpay-remit/utils"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB           *gorm.DB
	Cfg          *config.Config
	EmailService *services.EmailService
}

func NewAuthHandler(db *gorm.DB, cfg *config.Config, emailService *services.EmailService) *AuthHandler {
	return &AuthHandler{
		DB:           db,
		Cfg:          cfg,
		EmailService: emailService,
	}
}

// RegisterRequest is the request body for user registration.
type RegisterRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Name           string `json:"name" binding:"required"`
	Password       string `json:"password" binding:"required"`
	StellarAddress string `json:"stellar_address" binding:"required"`
	Country        string `json:"country"`
}

// LoginRequest is the request body for user login.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshTokenRequest is the request body for token refresh.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// ForgotPasswordRequest is the request body for forgot password.
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ResetPasswordRequest is the request body for password reset.
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// Register creates a new user account with a bcrypt-hashed password.
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	hash, err := models.HashPassword(req.Password)
	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"endpoint": "/auth/register",
		}).Warn("Registration rejected: weak password")
		c.Error(errors.NewValidationError("Invalid password", err.Error()))
		return
	}

	user := models.User{
		Email:          req.Email,
		Name:           req.Name,
		PasswordHash:   hash,
		StellarAddress: req.StellarAddress,
		Country:        req.Country,
	}

	if err := h.DB.Create(&user).Error; err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "UNIQUE") {
			c.Error(errors.NewConflictError("Email already registered"))
			return
		}
		logger.Log.WithFields(logrus.Fields{
			"endpoint": "/auth/register",
		}).Error("Failed to create user")
		c.Error(errors.NewInternalError("Failed to create user", err))
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"user_id":  user.ID,
		"endpoint": "/auth/register",
	}).Info("User registered")

	// Return the user object — PasswordHash is excluded via json:"-" on the model.
	c.JSON(http.StatusCreated, user)
}

// Login authenticates a user and returns JWT access and refresh tokens.
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	var user models.User
	if err := h.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.Error(errors.NewUnauthorizedError("Invalid credentials"))
		return
	}

	// Check if account is locked
	if user.IsAccountLocked() {
		logger.Log.WithFields(logrus.Fields{
			"user_id":  user.ID,
			"endpoint": "/auth/login",
		}).Warn("Login attempt on locked account")
		c.Error(errors.NewForbiddenError("Account is temporarily locked due to multiple failed login attempts. Please try again later."))
		return
	}

	if !user.IsActive {
		c.Error(errors.NewForbiddenError("User account is inactive"))
		return
	}

	if !models.ComparePassword(user.PasswordHash, req.Password) {
		// Record failed login attempt
		if err := user.RecordFailedLogin(h.DB); err != nil {
			logger.Log.WithFields(logrus.Fields{
				"user_id": user.ID,
			}).Error("Failed to record failed login attempt")
		}
		
		logger.Log.WithFields(logrus.Fields{
			"user_id":  user.ID,
			"endpoint": "/auth/login",
			"attempts": user.FailedLoginAttempts,
		}).Warn("Failed login attempt")
		
		// Check if account is now locked after recording the failed attempt
		if user.FailedLoginAttempts >= 5 {
			c.Error(errors.NewForbiddenError("Account locked due to multiple failed login attempts. Please try again in 30 minutes."))
		} else {
			c.Error(errors.NewUnauthorizedError("Invalid credentials"))
		}
		return
	}

	// Reset failed login attempts on successful login
	if err := user.ResetFailedLoginAttempts(h.DB); err != nil {
		logger.Log.WithFields(logrus.Fields{
			"user_id": user.ID,
		}).Error("Failed to reset login attempts")
	}

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

	logger.Log.WithFields(logrus.Fields{
		"user_id":  user.ID,
		"endpoint": "/auth/login",
	}).Info("User logged in")

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// Refresh validates a refresh token and issues new access and refresh tokens.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(req.RefreshToken, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.Cfg.JWTRefreshSecret), nil
	})

	if err != nil || !token.Valid {
		c.Error(errors.NewUnauthorizedError("Invalid or expired refresh token"))
		return
	}

	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil {
		c.Error(errors.NewUnauthorizedError("User not found"))
		return
	}

	if !user.IsActive {
		c.Error(errors.NewForbiddenError("User account is inactive"))
		return
	}

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


// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ForgotPassword initiates the password reset flow by sending a reset token via email
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	var user models.User
	if err := h.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// Don't reveal if email exists or not (security best practice)
		c.JSON(http.StatusOK, gin.H{
			"message": "If the email exists, a password reset link has been sent",
		})
		return
	}

	if !user.IsActive {
		// Don't reveal account status
		c.JSON(http.StatusOK, gin.H{
			"message": "If the email exists, a password reset link has been sent",
		})
		return
	}

	// Generate secure reset token (32 bytes = 64 hex characters)
	token, err := generateSecureToken(32)
	if err != nil {
		c.Error(errors.NewInternalError("Failed to generate reset token", err))
		return
	}

	// Set token expiration to 1 hour from now
	expiresAt := time.Now().Add(1 * time.Hour)
	user.ResetToken = token
	user.ResetTokenExpiresAt = &expiresAt

	if err := h.DB.Save(&user).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to save reset token", err))
		return
	}

	// Send password reset email
	if err := h.EmailService.SendPasswordResetEmail(&user, token); err != nil {
		logger.Log.WithFields(logrus.Fields{
			"user_id":  user.ID,
			"endpoint": "/auth/forgot-password",
		}).Error("Failed to send password reset email")
		// Don't fail the request if email fails - token is still valid
	}

	logger.Log.WithFields(logrus.Fields{
		"user_id":  user.ID,
		"endpoint": "/auth/forgot-password",
	}).Info("Password reset requested")

	c.JSON(http.StatusOK, gin.H{
		"message": "If the email exists, a password reset link has been sent",
	})
}

// ResetPassword completes the password reset flow using the token
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	var user models.User
	if err := h.DB.Where("reset_token = ?", req.Token).First(&user).Error; err != nil {
		c.Error(errors.NewUnauthorizedError("Invalid or expired reset token"))
		return
	}

	// Check if token has expired
	if user.ResetTokenExpiresAt == nil || time.Now().After(*user.ResetTokenExpiresAt) {
		c.Error(errors.NewUnauthorizedError("Invalid or expired reset token"))
		return
	}

	// Hash new password
	hash, err := models.HashPassword(req.NewPassword)
	if err != nil {
		c.Error(errors.NewValidationError("Invalid password", err.Error()))
		return
	}

	// Update password and clear reset token
	user.PasswordHash = hash
	user.ResetToken = ""
	user.ResetTokenExpiresAt = nil
	
	// Also reset any account lockout
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	user.LastFailedLoginAt = nil

	if err := h.DB.Save(&user).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to update password", err))
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"user_id":  user.ID,
		"endpoint": "/auth/reset-password",
	}).Info("Password reset successfully")

	})
}

// Logout invalidates the user's current token
func (h *AuthHandler) Logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
		return
	}

	tokenString := parts[1]
	claims := &middleware.Claims{}
	// Parse token without verifying since it already passed middleware
	token, _ := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.Cfg.JWTSecret), nil
	})

	if token != nil && claims.ID != "" && claims.ExpiresAt != nil {
		ttl := time.Until(claims.ExpiresAt.Time)
		if ttl > 0 {
			utils.SetCached("revoked:"+claims.ID, true, ttl)
		}
	}

	userID := uint(0)
	if claims != nil {
		userID = claims.UserID
	}

	logger.Log.WithFields(logrus.Fields{
		"user_id":  userID,
		"endpoint": "/auth/logout",
	}).Info("User logged out")

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}
