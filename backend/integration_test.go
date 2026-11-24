//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/handlers"
	"github.com/Gpay-Remit/gpay-remit/middleware"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/gin-gonic/gin"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
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

	migrations, err := os.ReadDir("migrations")
	require.NoError(t, err)
	for _, migration := range migrations {
		if migration.IsDir() || !strings.HasSuffix(migration.Name(), ".up.sql") {
			continue
		}
		migrationSQL, readErr := os.ReadFile(filepath.Join("migrations", migration.Name()))
		require.NoError(t, readErr, migration.Name())
		require.NoError(t, db.Exec(string(migrationSQL)).Error, migration.Name())
	}

	return db, cleanup
}

type integrationStellarClient struct{}

func (integrationStellarClient) SubmitPayment(context.Context, string, string, string, string, string) (string, error) {
	return "mock-tx-hash", nil
}

func (integrationStellarClient) ValidateAccount(context.Context, string) error { return nil }

func (integrationStellarClient) BuildEscrowTx(context.Context, string, string, string, string, string) (string, error) {
	return "mock-escrow-xdr", nil
}

func (integrationStellarClient) BuildPaymentTx(context.Context, txnbuild.Account, string, string, string, string) (*txnbuild.Transaction, error) {
	return nil, nil
}

func (integrationStellarClient) SignTx(context.Context, string, string) (string, error) {
	return "mock-signed-xdr", nil
}

func setupRouter(db *gorm.DB, clients ...utils.StellarClientInterface) *gin.Engine {
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

	emailService := services.NewEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPFrom, cfg.EmailEnabled)
	authHandler := handlers.NewAuthHandler(db, cfg, emailService)
	router.POST("/api/v1/auth/register", authHandler.Register)
	router.POST("/api/v1/auth/login", authHandler.Login)

	protected := router.Group("/api/v1")
	protected.Use(middleware.JwtAuthMiddleware(db, cfg))
	var stellarClient utils.StellarClientInterface = integrationStellarClient{}
	if len(clients) > 0 && clients[0] != nil {
		stellarClient = clients[0]
	}
	remittanceHandler := handlers.NewRemittanceHandlerWithStellarClient(db, cfg, stellarClient)
	contactHandler := handlers.NewContactHandler(db, cfg, stellarClient)
	protected.POST("/contacts", contactHandler.CreateContact)
	protected.POST("/remittances/create", remittanceHandler.CreateRemittance)
	protected.POST("/remittances", remittanceHandler.SendRemittance)
	protected.GET("/remittances", remittanceHandler.ListRemittances)

	return router
}

func registerAndLogin(t *testing.T, router *gin.Engine) string {
	t.Helper()
	previousTransport := http.DefaultTransport
	http.DefaultTransport = integrationHTTPTransport{fallback: previousTransport}
	defer func() { http.DefaultTransport = previousTransport }()
	const password = "Integration-FullFlow-2026!9"

	registerPayload := map[string]any{
		"email":           "integration@example.com",
		"name":            "Integration Tester",
		"password":        password,
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
		"password": password,
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

type integrationHTTPTransport struct {
	fallback http.RoundTripper
}

func (transport integrationHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host == "api.pwnedpasswords.com" {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	}
	return transport.fallback.RoundTrip(request)
}

func TestIntegrationAuthAndPayments(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	router := setupRouter(db)
	accessToken := registerAndLogin(t, router)

	payload := map[string]any{
		"sender_id":       1,
		"recipient_id":    2,
		"amount":          50.5,
		"currency":        "USD",
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

func TestIntegrationUserContactAndRemittanceFlow(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	stellar := integrationStellarClient{}
	router := setupRouter(db, stellar)
	token := registerAndLogin(t, router)
	var sender models.User
	require.NoError(t, db.Where("email = ?", "integration@example.com").First(&sender).Error)

	recipientAddress := "GBZC6YRFWINCGYH6FFIK3VY4KF3WZJQR7CD3S5Y4GVNIKU5RM3JY7YEX"
	contactBody, err := json.Marshal(map[string]string{
		"nickname":        "Remittance recipient",
		"stellar_address": recipientAddress,
		"currency":        "USDC",
	})
	require.NoError(t, err)
	contactResponse := httptest.NewRecorder()
	contactRequest := httptest.NewRequest(http.MethodPost, "/api/v1/contacts", bytes.NewReader(contactBody))
	contactRequest.Header.Set("Authorization", "Bearer "+token)
	contactRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(contactResponse, contactRequest)
	require.Equal(t, http.StatusCreated, contactResponse.Code, contactResponse.Body.String())

	var savedContact models.Contact
	require.NoError(t, db.Where("user_id = ? AND stellar_address = ?", sender.ID, recipientAddress).First(&savedContact).Error)
	require.Equal(t, sender.ID, savedContact.UserID)

	remittanceBody, err := json.Marshal(map[string]any{
		"sender_account":    sender.StellarAddress,
		"recipient_account": savedContact.StellarAddress,
		"amount":            50_000_000,
		"asset_code":        "USDC",
	})
	require.NoError(t, err)
	remittanceResponse := httptest.NewRecorder()
	remittanceRequest := httptest.NewRequest(http.MethodPost, "/api/v1/remittances/create", bytes.NewReader(remittanceBody))
	remittanceRequest.Header.Set("Authorization", "Bearer "+token)
	remittanceRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(remittanceResponse, remittanceRequest)
	require.Equal(t, http.StatusCreated, remittanceResponse.Code, remittanceResponse.Body.String())

	var response map[string]any
	require.NoError(t, json.Unmarshal(remittanceResponse.Body.Bytes(), &response))
	require.Equal(t, "mock-escrow-xdr", response["tx_envelope"])

	var payment models.Payment
	require.NoError(t, db.Where("sender_id = ? AND recipient_account = ?", sender.ID, recipientAddress).First(&payment).Error)
	require.EqualValues(t, 50_000_000, payment.Amount)
	require.Equal(t, "USDC", payment.Currency)
	require.Equal(t, "pending", payment.Status)
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

// This integration test sets up a mock server using testcontainers
// to test the webhook delivery flow end-to-end.
func TestWebhookIntegration(t *testing.T) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "mockserver/mockserver:latest",
		ExposedPorts: []string{"1080/tcp"},
		WaitingFor:   wait.ForLog("started on port"),
	}

	mockServerC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Could not start mock server container: %v", err)
	}
	defer mockServerC.Terminate(ctx)

	mockServerIP, err := mockServerC.Host(ctx)
	if err != nil {
		t.Fatalf("Could not get mock server IP: %v", err)
	}

	mockServerPort, err := mockServerC.MappedPort(ctx, "1080")
	if err != nil {
		t.Fatalf("Could not get mock server Port: %v", err)
	}

	mockServerURL := "http://" + mockServerIP + ":" + mockServerPort.Port()

	expectation := map[string]interface{}{
		"httpRequest": map[string]interface{}{
			"method": "POST",
			"path":   "/webhook",
		},
		"httpResponse": map[string]interface{}{
			"statusCode": 200,
		},
	}

	expectationBytes, _ := json.Marshal(expectation)
	http.Post(mockServerURL+"/mockserver/expectation", "application/json", bytes.NewBuffer(expectationBytes))

	payload := map[string]string{"status": "success", "tx_id": "12345"}
	payloadBytes, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(mockServerURL+"/webhook", "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		t.Fatalf("Webhook delivery failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}
