package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/logger"
	"github.com/yourusername/gpay-remit/middleware"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB  *gorm.DB
	Cfg *config.Config
}

func NewAuthHandler(db *gorm.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{DB: db, Cfg: cfg}
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

// Register creates a new user account with a bcrypt-hashed password.
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := models.HashPassword(req.Password)
	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"endpoint": "/auth/register",
		}).Warn("Registration rejected: weak password")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		logger.Log.WithFields(logrus.Fields{
			"endpoint": "/auth/register",
		}).Error("Failed to create user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user models.User
	if err := h.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "User account is inactive"})
		return
	}

	if !models.ComparePassword(user.PasswordHash, req.Password) {
		logger.Log.WithFields(logrus.Fields{
			"user_id":  user.ID,
			"endpoint": "/auth/login",
		}).Warn("Failed login attempt")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	accessToken, err := middleware.GenerateToken(user.ID, user.Role, h.Cfg.JWTSecret, 15*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	refreshToken, err := middleware.GenerateToken(user.ID, user.Role, h.Cfg.JWTRefreshSecret, 7*24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(req.RefreshToken, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.Cfg.JWTRefreshSecret), nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired refresh token", "code": "InvalidToken"})
		return
	}

	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
		return
	}

	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "User account is inactive"})
		return
	}

	accessToken, err := middleware.GenerateToken(user.ID, user.Role, h.Cfg.JWTSecret, 15*time.Minute)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	refreshToken, err := middleware.GenerateToken(user.ID, user.Role, h.Cfg.JWTRefreshSecret, 7*24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}
