//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/handlers"
	"github.com/yourusername/gpay-remit/middleware"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	t := t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		Env:          map[string]string{"POSTGRES_USER": "test", "POSTGRES_PASSWORD": "test", "POSTGRES_DB": "testdb"},
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor:   wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	cleanup := func() {
		require.NoError(t, container.Terminate(ctx))
	}

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := fmt.Sprintf("host=%s port=%s user=test password=test dbname=testdb sslmode=disable", host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		sqlDB, err := db.DB()
		if err != nil {
			return false
		}
		return sqlDB.Ping() == nil
	}, 30*time.Second, 500*time.Millisecond)

	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Payment{}, &models.Invoice{}, &models.Webhook{}, &models.WebhookDelivery{}, &models.IdempotencyRecord{}))

	return db, cleanup
}

func setupRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		DatabaseURL:       "",
		HorizonURL:        "https://horizon-testnet.stellar.org",
		NetworkPassphrase: "Test SDF Network ; September 2015",
		JWTSecret:         "test-secret",
		JWTRefreshSecret:  "test-refresh-secret",
	}

	router := gin.New()
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.ErrorHandler())
	router.Use(middleware.VersionMiddleware())

	authHandler := handlers.NewAuthHandler(db, cfg)
	router.POST("/api/v1/auth/register", authHandler.Register)
	router.POST("/api/v1/auth/login", authHandler.Login)

	protected := router.Group("/api/v1")
	protected.Use(middleware.JwtAuthMiddleware(cfg))
	remittanceHandler := handlers.NewRemittanceHandler(db, cfg)
	protected.POST("/remittances", remittanceHandler.SendRemittance)
	protected.GET("/remittances", remittanceHandler.ListRemittances)

	return router
}

func registerAndLogin(t *testing.T, router *gin.Engine) string {
	registerPayload := map[string]any{
		"email":           "integration@example.com",
		"name":            "Integration Tester",
		"password":        "securepass123",
		"stellar_address": "GCFXW67O6JYQXZF3ZKCRNGEF6W6C64KP3S2ZMLHJH33A6YSFWTQZ7Q2Z",
		"country":         "US",
	}
	body, err := json.Marshal(registerPayload)
	require.NoError(t, err)

	r := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(r, req)
	require.Equal(t, http.StatusCreated, r.Code)

	loginPayload := map[string]any{
		"email":    "integration@example.com",
		"password": "securepass123",
	}
	body, err = json.Marshal(loginPayload)
	require.NoError(t, err)

	r = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(r, req)
	require.Equal(t, http.StatusOK, r.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &resp))
	require.NotEmpty(t, resp["access_token"])

	return resp["access_token"].(string)
}

func TestIntegrationAuthAndPayments(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	router := setupRouter(db)
	accessToken := registerAndLogin(t, router)

	payload := map[string]any{
		"sender_id":    1,
		"recipient_id": 2,
		"amount":       50.5,
		"currency":     "USD",
		"target_currency": "EUR",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	r := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/remittances", bytes.NewReader(body))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(r, req)
	require.Equal(t, http.StatusCreated, r.Code)

	var payResp map[string]any
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &payResp))
	require.Equal(t, "pending", payResp["status"])
	require.Equal(t, 50.5, payResp["amount"])

	// verify persistence
	var payment models.Payment
	require.NoError(t, db.First(&payment).Error)
	require.Equal(t, float64(50.5), payment.Amount)
	require.Equal(t, "pending", payment.Status)

	// list remittances and ensure inclusion
	r = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/remittances", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	router.ServeHTTP(r, req)
	require.Equal(t, http.StatusOK, r.Code)

	var listResp []map[string]any
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &listResp))
	require.NotEmpty(t, listResp)
}

func TestIntegrationStandardizedErrors(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	router := setupRouter(db)

	invalidPayload := map[string]any{"email": "bad-email", "password": "x"}
	body, err := json.Marshal(invalidPayload)
	require.NoError(t, err)

	r := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(r, req)
	require.Equal(t, http.StatusBadRequest, r.Code)

	var responseBody map[string]any
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &responseBody))
	require.Contains(t, responseBody, "error")
	errPayload := responseBody["error"].(map[string]any)
	require.Equal(t, "VALIDATION_ERROR", errPayload["code"])
	require.NotEmpty(t, errPayload["message"])
}
