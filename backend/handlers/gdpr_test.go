package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGDPRTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.User{},
		&models.Payment{},
		&models.PaymentRequest{},
		&models.Invoice{},
		&models.Contact{},
		&models.Webhook{},
		&models.AuditLog{},
	))
	return db
}

// seedGDPRUser creates a user with one row of every personal-data type that
// the export must cover, plus an unrelated "other" user whose data must not
// leak into the export.
func seedGDPRUser(t *testing.T, db *gorm.DB) (userID uint, otherID uint) {
	t.Helper()

	user := models.User{Email: "subject@example.com", Name: "Subject", StellarAddress: "GSUBJECT", PasswordHash: "x"}
	other := models.User{Email: "other@example.com", Name: "Other", StellarAddress: "GOTHER", PasswordHash: "x"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&other).Error)

	payment := models.Payment{SenderID: user.ID, RecipientID: other.ID, Amount: 1000, Currency: "USD", Status: "completed"}
	require.NoError(t, db.Create(&payment).Error)

	// A payment between two other users, which must not be exported for the subject.
	otherPayment := models.Payment{SenderID: other.ID, RecipientID: other.ID, Amount: 5, Currency: "USD"}
	require.NoError(t, db.Create(&otherPayment).Error)

	require.NoError(t, db.Create(&models.PaymentRequest{
		RequesterID: user.ID, TargetUserID: other.ID, Amount: 200, Currency: "USD", AssetCode: "USDC", Status: "pending",
	}).Error)

	dueDate := time.Now().Add(72 * time.Hour)
	require.NoError(t, db.Create(&models.Invoice{
		PaymentID: payment.ID, InvoiceNo: "INV-GDPR-1", IssuerID: other.ID, RecipientID: user.ID,
		Amount: 1000, Currency: "USD", DueDate: &dueDate, Status: "unpaid",
	}).Error)

	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Nickname: "Mum", StellarAddress: "GCONTACTADDRESS", Email: "mum@example.com",
		Notes: "birthday 3 May", VerificationStatus: models.ContactVerificationVerified, IsVerified: true,
	}).Error)

	require.NoError(t, db.Create(&models.Webhook{
		UserID: user.ID, URL: "https://example.com/hook", Secret: "super-secret-signing-key",
		Events: models.WebhookEvents{"payment.completed"}, IsActive: true, Description: "ledger sync",
	}).Error)

	userIDPtr := user.ID
	require.NoError(t, db.Create(&models.AuditLog{
		UserID: &userIDPtr, Action: "POST", Resource: "/api/v1/remittances", IPAddress: "10.0.0.1", ResponseStatus: 201,
	}).Error)

	return user.ID, other.ID
}

func runGDPRExport(t *testing.T, db *gorm.DB, userID uint) (*httptest.ResponseRecorder, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := NewGDPRHandler(db)

	router := gin.New()
	router.Use(errorHandlerForTests())
	router.Use(func(c *gin.Context) {
		c.Set("userID", userID)
		c.Next()
	})
	router.GET("/gdpr/export", handler.ExportUserData)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/gdpr/export", nil)
	router.ServeHTTP(w, req)
	return w, w.Body.String()
}

func TestGDPRExport_IncludesAllDataTypes(t *testing.T) {
	db := setupGDPRTestDB(t)
	userID, _ := seedGDPRUser(t, db)

	w, body := runGDPRExport(t, db, userID)
	require.Equal(t, http.StatusOK, w.Code, body)

	var resp struct {
		User            map[string]interface{}   `json:"user"`
		Payments        []map[string]interface{} `json:"payments"`
		PaymentRequests []map[string]interface{} `json:"payment_requests"`
		Invoices        []map[string]interface{} `json:"invoices"`
		Contacts        []map[string]interface{} `json:"contacts"`
		Webhooks        []map[string]interface{} `json:"webhooks"`
		AuditLogs       []map[string]interface{} `json:"audit_logs"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &resp))

	assert.Equal(t, "subject@example.com", resp.User["email"])

	// Payments: only the one the subject sent, not the unrelated one.
	require.Len(t, resp.Payments, 1)
	assert.Equal(t, float64(1000), resp.Payments[0]["amount"])

	require.Len(t, resp.PaymentRequests, 1)
	assert.Equal(t, "pending", resp.PaymentRequests[0]["status"])

	require.Len(t, resp.Invoices, 1)
	assert.Equal(t, "INV-GDPR-1", resp.Invoices[0]["invoice_no"])
	assert.NotContains(t, resp.Invoices[0], "payment", "embedded payment must not be emitted as a zero-value object")

	require.Len(t, resp.Contacts, 1)
	assert.Equal(t, "Mum", resp.Contacts[0]["nickname"])
	assert.Equal(t, "verified", resp.Contacts[0]["verification_status"])
	assert.Equal(t, "mum@example.com", resp.Contacts[0]["email"])
	assert.Equal(t, "birthday 3 May", resp.Contacts[0]["notes"])

	require.Len(t, resp.Webhooks, 1)
	assert.Equal(t, "https://example.com/hook", resp.Webhooks[0]["url"])
	assert.Equal(t, "payment.completed", resp.Webhooks[0]["events"])

	// The subject's own export audit entries are included; the export itself
	// is recorded after the snapshot is taken, so it is not in this response.
	require.Len(t, resp.AuditLogs, 1)
	assert.Equal(t, "/api/v1/remittances", resp.AuditLogs[0]["resource"])
	assert.Equal(t, float64(201), resp.AuditLogs[0]["response_status"])
	assert.Equal(t, "10.0.0.1", resp.AuditLogs[0]["ip_address"])
}

func TestGDPRExport_OmitsWebhookSecrets(t *testing.T) {
	db := setupGDPRTestDB(t)
	userID, _ := seedGDPRUser(t, db)

	_, body := runGDPRExport(t, db, userID)

	assert.False(t, strings.Contains(body, "super-secret-signing-key"), "webhook signing secret leaked into export")
	assert.NotContains(t, body, `"secret"`)
	assert.NotContains(t, body, `"password_hash"`)
}

func TestGDPRExport_ExcludesOtherUsersData(t *testing.T) {
	db := setupGDPRTestDB(t)
	_, otherID := seedGDPRUser(t, db)

	// Export as the other user: they own 1 self-payment and 1 payment request target,
	// and nothing of the subject's invoices/contacts/webhooks.
	w, body := runGDPRExport(t, db, otherID)
	require.Equal(t, http.StatusOK, w.Code, body)

	var resp struct {
		Payments        []map[string]interface{} `json:"payments"`
		PaymentRequests []map[string]interface{} `json:"payment_requests"`
		Invoices        []map[string]interface{} `json:"invoices"`
		Contacts        []map[string]interface{} `json:"contacts"`
		Webhooks        []map[string]interface{} `json:"webhooks"`
		AuditLogs       []map[string]interface{} `json:"audit_logs"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &resp))

	// The other user is the issuer of the invoice, so they see it.
	assert.Len(t, resp.Invoices, 1)
	// But they own no contacts, webhooks or subject-only audit logs.
	assert.Empty(t, resp.Contacts)
	assert.Empty(t, resp.Webhooks)
	assert.Empty(t, resp.AuditLogs)
	// Payments: the subject's payment (sender=subject, recipient=other) is visible to other as recipient,
	// plus their own self-payment.
	assert.Len(t, resp.Payments, 2)
}

func TestGDPRExport_UnauthenticatedRejected(t *testing.T) {
	db := setupGDPRTestDB(t)
	gin.SetMode(gin.TestMode)
	handler := NewGDPRHandler(db)

	router := gin.New()
	router.Use(errorHandlerForTests())
	router.GET("/gdpr/export", handler.ExportUserData)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/gdpr/export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
