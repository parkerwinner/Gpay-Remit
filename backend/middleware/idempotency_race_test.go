package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupIdempotencyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Each SQLite ":memory:" connection is its own independent, empty
	// database — under gorm's default connection pool, concurrent
	// goroutines can end up on different connections that never saw
	// AutoMigrate's CREATE TABLE. Force a single shared connection so this
	// test exercises real concurrent access to one database, not N
	// disjoint ones.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.IdempotencyRecord{}))
	return db
}

func buildIdempotencyTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(IdempotencyMiddleware(db))
	router.POST("/api/v1/remittances", func(c *gin.Context) {
		// Simulate real handler work, widening the window in which two
		// concurrent requests can both observe "no existing record yet".
		SetIdempotencyResponse(c, gin.H{"status": "created"})
		c.JSON(http.StatusCreated, gin.H{"status": "created"})
	})
	return router
}

// TestIdempotencyMiddleware_ConcurrentRequestsWithSameKey_NeverCreateDuplicateRecords
// is the regression test for #195: N goroutines send the same
// Idempotency-Key + body simultaneously. Before the fix (check-then-act
// with no DB-level uniqueness), this could create more than one
// idempotency_records row for the same key. After the fix
// (uniqueIndex + graceful handling of the resulting constraint violation),
// exactly one row must exist no matter how many requests race.
func TestIdempotencyMiddleware_ConcurrentRequestsWithSameKey_NeverCreateDuplicateRecords(t *testing.T) {
	db := setupIdempotencyTestDB(t)
	router := buildIdempotencyTestRouter(db)

	const concurrentRequests = 20
	const idempotencyKey = "race-test-key-0123456789abcdef"
	const body = `{"amount":"100"}`

	var wg sync.WaitGroup
	statusCodes := make([]int, concurrentRequests)

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/api/v1/remittances", strings.NewReader(body))
			req.Header.Set("Idempotency-Key", idempotencyKey)
			router.ServeHTTP(w, req)
			statusCodes[idx] = w.Code
		}(i)
	}
	wg.Wait()

	var count int64
	require.NoError(t, db.Model(&models.IdempotencyRecord{}).
		Where("idempotency_key = ?", idempotencyKey).
		Count(&count).Error)

	assert.Equal(t, int64(1), count, "exactly one idempotency_records row must exist for the key, regardless of how many requests raced for it")

	// Every request must have received a real HTTP response (no request
	// should be left hanging or crash the handler goroutine).
	for i, code := range statusCodes {
		assert.NotZero(t, code, "request %d did not receive a response", i)
	}
}

func TestIdempotencyMiddleware_DifferentKeys_EachCreateTheirOwnRecord(t *testing.T) {
	db := setupIdempotencyTestDB(t)
	router := buildIdempotencyTestRouter(db)

	for _, key := range []string{"key-aaaaaaaaaaaaaaaaaaaa", "key-bbbbbbbbbbbbbbbbbbbb"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/remittances", strings.NewReader(`{"amount":"1"}`))
		req.Header.Set("Idempotency-Key", key)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
	}

	var count int64
	require.NoError(t, db.Model(&models.IdempotencyRecord{}).Count(&count).Error)
	assert.Equal(t, int64(2), count, "distinct keys must not collide with each other")
}
