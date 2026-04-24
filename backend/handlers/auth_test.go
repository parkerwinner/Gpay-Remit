package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/models"
)

func setupAuthHandler(t *testing.T) (*AuthHandler, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupTestDB()
	cfg := &config.Config{
		JWTSecret:        "test-secret",
		JWTRefreshSecret: "test-refresh-secret",
	}
	handler := NewAuthHandler(db, cfg)
	router := gin.New()
	router.POST("/auth/register", handler.Register)
	router.POST("/auth/login", handler.Login)
	router.POST("/auth/refresh", handler.Refresh)
	return handler, router
}

func TestRegister(t *testing.T) {
	_, router := setupAuthHandler(t)

	t.Run("Valid Registration", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":           "test@example.com",
			"name":            "Test User",
			"password":        "Secure@123",
			"stellar_address": "GDQJUTQYK2MQX2VGDR2FYWLIYAQIEGXTQVTFEMGH6DNHFMHIDENFINMJTEST",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.Equal(t, "test@example.com", resp["email"])
		assert.Nil(t, resp["password_hash"])
	})

	t.Run("Duplicate Email Returns 409", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":           "dup@example.com",
			"name":            "First User",
			"password":        "Secure@123",
			"stellar_address": "GDQJUTQYK2MQX2VGDR2FYWLIYAQIEGXTQVTFEMGH6DNHFMHIDENFINDUP1",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)

		// Second registration with same email
		body2, _ := json.Marshal(map[string]string{
			"email":           "dup@example.com",
			"name":            "Second User",
			"password":        "Secure@456",
			"stellar_address": "GDQJUTQYK2MQX2VGDR2FYWLIYAQIEGXTQVTFEMGH6DNHFMHIDENFINDUP2",
		})
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(body2))
		req2.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusConflict, w2.Code)
	})

	t.Run("Weak Password - Too Short", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":           "weak@example.com",
			"name":            "Weak User",
			"password":        "abc",
			"stellar_address": "GDQJUTQYK2MQX2VGDR2FYWLIYAQIEGXTQVTFEMGH6DNHFMHIDENFINWEAK",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Weak Password - No Special Character", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":           "nospecial@example.com",
			"name":            "No Special",
			"password":        "Secure123",
			"stellar_address": "GDQJUTQYK2MQX2VGDR2FYWLIYAQIEGXTQVTFEMGH6DNHFMHIDENFINNSP",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "special character")
	})

	t.Run("Missing Required Fields", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email": "incomplete@example.com",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid Email Format", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":           "not-an-email",
			"name":            "Bad Email",
			"password":        "Secure@123",
			"stellar_address": "GDQJUTQYK2MQX2VGDR2FYWLIYAQIEGXTQVTFEMGH6DNHFMHIDENFINBAD",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestLogin(t *testing.T) {
	_, router := setupAuthHandler(t)

	// Pre-register a user
	registerBody, _ := json.Marshal(map[string]string{
		"email":           "login@example.com",
		"name":            "Login User",
		"password":        "Secure@Login1",
		"stellar_address": "GDQJUTQYK2MQX2VGDR2FYWLIYAQIEGXTQVTFEMGH6DNHFMHIDENFINLOGIN",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(registerBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	t.Run("Valid Credentials Return Tokens", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":    "login@example.com",
			"password": "Secure@Login1",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NotEmpty(t, resp["access_token"])
		assert.NotEmpty(t, resp["refresh_token"])
	})

	t.Run("Wrong Password Returns 401", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":    "login@example.com",
			"password": "WrongPassword@1",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid credentials")
	})

	t.Run("Unknown Email Returns 401", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"email":    "nobody@example.com",
			"password": "Secure@123",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Missing Email Field", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"password": "Secure@123",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Empty Body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Inactive User Returns 403", func(t *testing.T) {
		handler, r := setupAuthHandler(t)

		// Create inactive user directly in DB
		hash, _ := models.HashPassword("Secure@Inactive1")
		user := models.User{
			Email:          "inactive@example.com",
			Name:           "Inactive User",
			PasswordHash:   hash,
			StellarAddress: "GDQJUTQYK2MQX2VGDR2FYWLIYAQIEGXTQVTFEMGH6DNHFMHIDENFINIAC",
			IsActive:       false,
		}
		handler.DB.Create(&user)

		body, _ := json.Marshal(map[string]string{
			"email":    "inactive@example.com",
			"password": "Secure@Inactive1",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}
